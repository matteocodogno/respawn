// respawn: comic-book slide decks as code. Write them in Markdown, present them with live
// terminals, and run the room with polls, questions, breaks, exercises and a follow-along
// audience copy.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/matteocodogno/respawn"
	"github.com/matteocodogno/respawn/internal/deck"
	"github.com/matteocodogno/respawn/internal/server"
)

// version is set at release time (-ldflags "-X main.version=0.1.0").
var version = "dev"

const usage = `respawn: comic-book slide decks as code.

Usage:
  respawn new <folder>                 start a presentation (a copy of the showcase)
  respawn serve [deck] [flags]         present a deck (default: the first one)
  respawn build [--out folder]         check every deck; --out writes what was built
  respawn pdf [deck] --from N --to M   a range of slides as a 16:9 PDF (--mode text: selectable text)
  respawn version

Every command works on the presentation in the current folder (the one with
respawn.yaml); --dir picks another one.

serve flags:
  --port N        presenter's port (default: the deck's port in respawn.yaml)
  --dev           rebuild and reload the browser while you edit the slides
  --audience      also serve the room's read-only copy on port+2 (LAN)
  --tunnel        --audience through the Cloudflare Tunnel of respawn.yaml
  --resume        keep the last session code and the room's slide
  --session CODE  session code by hand
  --live N        the room's slide at start

https://github.com/matteocodogno/respawn
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "new":
		err = cmdNew(args)
	case "serve", "present", "run":
		err = cmdServe(args)
	case "build", "check":
		err = cmdBuild(args)
	case "pdf":
		err = cmdPDF(args)
	case "version", "--version", "-v":
		fmt.Println("respawn", version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "respawn: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "respawn:", err)
		os.Exit(1)
	}
}

// parse accepts the deck name before or after the flags.
func parse(fs *flag.FlagSet, args []string) (string, error) {
	pos := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pos, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if rest := fs.Args(); len(rest) > 0 {
		if pos != "" || len(rest) > 1 {
			return "", fmt.Errorf("unexpected arguments: %s", strings.Join(rest, " "))
		}
		pos = rest[0]
	}
	return pos, nil
}

func load(dir string) (*deck.Project, error) {
	p, err := deck.Load(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no %s in %s: run respawn new <folder>, or pass --dir", deck.ConfigFile, dir)
		}
		return nil, err
	}
	return p, p.Config.CheckVersion(version)
}

func cmdServe(args []string) error {
	f := flag.NewFlagSet("serve", flag.ExitOnError)
	o := server.Options{}
	f.StringVar(&o.Dir, "dir", ".", "presentation folder")
	f.StringVar(&o.Host, "host", "localhost", "presenter's address")
	f.IntVar(&o.Port, "port", 0, "presenter's port")
	f.BoolVar(&o.Dev, "dev", false, "live reload")
	f.BoolVar(&o.Audience, "audience", false, "serve the room's copy on port+2")
	f.BoolVar(&o.Tunnel, "tunnel", false, "open the Cloudflare Tunnel too")
	f.BoolVar(&o.Resume, "resume", false, "keep the last session code and slide")
	f.StringVar(&o.Session, "session", "", "session code")
	f.IntVar(&o.Live, "live", -1, "the room's slide at start")
	d, err := parse(f, args)
	if err != nil {
		return err
	}
	o.Deck = d
	if o.Resume || o.Session != "" || o.Live >= 0 {
		o.Audience = true
	}
	if _, err := load(o.Dir); err != nil {
		return err
	}
	s, err := server.New(o)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return s.Run(ctx)
}

func cmdBuild(args []string) error {
	f := flag.NewFlagSet("build", flag.ExitOnError)
	dir := f.String("dir", ".", "presentation folder")
	out := f.String("out", "", "write the built decks, search indexes and catalog here")
	if _, err := parse(f, args); err != nil {
		return err
	}
	p, err := load(*dir)
	if err != nil {
		return err
	}
	for _, d := range p.Config.Decks {
		b := p.Decks[d.Name]
		fmt.Printf("%-12s %4d slides, %3d with a terminal, %2d exercises\n", d.Name, len(b.Slides), len(b.Terms), len(b.Catalog))
	}
	fmt.Printf("%-12s %4d exercises in the shared catalog\n", "catalog", len(p.Catalog))
	if err := server.CheckTheme(p.Config); err != nil {
		return err
	}
	theme := p.Config.Theme
	if theme == "" {
		theme = server.DefaultTheme
	}
	fmt.Printf("%-12s %s (available: %s)\n", "theme", theme, strings.Join(server.Themes(p.Config), ", "))
	if *out != "" {
		if err := p.WriteDebug(*out); err != nil {
			return err
		}
		fmt.Println("written to", *out)
	}
	return nil
}

func cmdPDF(args []string) error {
	f := flag.NewFlagSet("pdf", flag.ExitOnError)
	dir := f.String("dir", ".", "presentation folder")
	from := f.Int("from", 0, "first slide")
	to := f.Int("to", -1, "last slide (default: the last one)")
	out := f.String("out", "", "output file (default: in ~/Downloads, named after the module)")
	theme := f.String("theme", "", "halloween")
	mode := f.String("mode", "exact", "exact: the slides as on screen (images); text: vector, selectable text")
	d, err := parse(f, args)
	if err != nil {
		return err
	}
	if _, err := load(*dir); err != nil {
		return err
	}
	s, err := server.New(server.Options{Dir: *dir, Deck: d, Host: "127.0.0.1", Out: os.Stderr})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	data, name, err := s.ExportPDF(ctx, *from, *to, *theme, *mode)
	if err != nil {
		return err
	}
	path := *out
	if path == "" {
		home, _ := os.UserHomeDir()
		dl := filepath.Join(home, "Downloads")
		if st, err := os.Stat(dl); err != nil || !st.IsDir() {
			dl = "."
		}
		path = filepath.Join(dl, name)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func cmdNew(args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: respawn new <folder>")
	}
	dst := args[0]
	if entries, err := os.ReadDir(dst); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty", dst)
	}
	src, _ := fs.Sub(respawn.Starter, "examples/showcase")
	err := fs.WalkDir(src, ".", func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(path))
		if e.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return err
	}
	for _, extra := range []string{"assets/images", "assets/media", "workspace"} {
		_ = os.MkdirAll(filepath.Join(dst, extra), 0o755)
	}
	_ = os.WriteFile(filepath.Join(dst, ".gitignore"), []byte(".respawn/\nworkspace/\n"), 0o644)
	fmt.Printf("A new presentation in %s. Next:\n  cd %s\n  respawn serve --dev\n", dst, dst)
	return nil
}
