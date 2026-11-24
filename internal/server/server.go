// Package server is `respawn serve`: the presenter's deck on localhost (slides, terminals,
// notes, PDF export, live reload) and, on demand, the audience copy and the tunnel.
package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/matteocodogno/respawn/internal/audience"
	"github.com/matteocodogno/respawn/internal/deck"
	"github.com/matteocodogno/respawn/internal/livereload"
	"github.com/matteocodogno/respawn/internal/pdf"
	"github.com/matteocodogno/respawn/internal/terminal"
	"github.com/matteocodogno/respawn/internal/tunnel"
)

//go:embed index.tmpl.html
var indexHTML string

// Options are the settings of `respawn serve`.
type Options struct {
	Dir      string // the presentation (folder of respawn.yaml)
	Deck     string // deck name; default the first one
	Host     string // presenter's address, default localhost
	Port     int    // default the deck's port in respawn.yaml
	Dev      bool   // rebuild and reload the browser when a slide changes
	Audience bool   // also serve the room's copy on Port+2
	Tunnel   bool   // also open the Cloudflare Tunnel (implies Audience)
	Resume   bool   // keep the last session code and the room's slide
	Session  string // session code by hand
	Live     int    // room's slide at start, -1 for the default
	Out      io.Writer
}

// Server is one deck being presented.
type Server struct {
	o      Options
	dir    string
	tmpl   *template.Template
	mu     sync.RWMutex
	proj   *deck.Project
	built  *deck.Built
	pages  map[int]string
	assets *overlay
	hasher *hasher
	static http.Handler
	term   *terminal.Terminals
	aud    *audience.Server
	pdfMu  sync.Mutex
}

// New loads the presentation and builds the deck.
func New(o Options) (*Server, error) {
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Tunnel {
		o.Audience = true
	}
	if o.Host == "" {
		o.Host = "localhost"
	}
	proj, err := deck.Load(o.Dir)
	if err != nil {
		return nil, err
	}
	cfg := proj.Config
	if o.Deck == "" {
		o.Deck = cfg.Decks[0].Name
	}
	d, err := cfg.DeckByName(o.Deck)
	if err != nil {
		return nil, err
	}
	if o.Port == 0 {
		o.Port = d.Port
	}
	s := &Server{o: o, dir: cfg.Dir, proj: proj, built: proj.Decks[o.Deck], pages: map[int]string{}}
	s.assets = newOverlay(cfg.Path(cfg.Assets))
	s.hasher = &hasher{fs: s.assets, keep: !o.Dev}
	s.static = http.FileServer(http.FS(s.assets))
	s.tmpl = template.Must(template.New("index").Funcs(template.FuncMap{"hash": s.hasher.hash}).Parse(indexHTML))
	s.term = &terminal.Terminals{
		Root:   cfg.Dir,
		Bashrc: filepath.Join(cfg.Path(cfg.Assets), ".bashrc"),
		History: func() []string {
			s.mu.RLock()
			defer s.mu.RUnlock()
			return s.proj.History
		},
	}
	return s, nil
}

// project returns the current build.
func (s *Server) current() (*deck.Project, *deck.Built) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.proj, s.built
}

// rebuild reloads respawn.yaml and the slides (--dev).
func (s *Server) rebuild() error {
	proj, err := deck.Load(s.dir)
	if err != nil {
		return err
	}
	b, ok := proj.Decks[s.o.Deck]
	if !ok {
		return fmt.Errorf("deck %q is not in respawn.yaml any more", s.o.Deck)
	}
	s.mu.Lock()
	s.proj, s.built, s.pages = proj, b, map[int]string{}
	s.mu.Unlock()
	return nil
}

type page struct {
	HTML        template.HTML
	Lang        string
	Title       string
	Config      any
	PrevURL     string
	NextURL     string
	CurrentStep int
	StepCount   int
	DevMode     bool
}

func stepURL(i int) string {
	if i == 0 {
		return "/"
	}
	return "/" + strconv.Itoa(i)
}

// runtimeConfig is window.RESPAWN: what assets/js/respawn.js needs from respawn.yaml.
func (s *Server) runtimeConfig(p *deck.Project, b *deck.Built) map[string]any {
	cfg := p.Config
	labels := map[string]string{}
	for _, d := range cfg.Decks {
		label := d.Label
		if label == "" {
			label = strings.ToUpper(d.Name)
		}
		labels[d.Name] = label
	}
	strs := cfg.Strings
	if strs == nil {
		strs = map[string]string{}
	}
	return map[string]any{
		"ns": cfg.Namespace, "lang": cfg.Lang, "gif": cfg.Gif, "strings": strs,
		"brand": cfg.Brand, "title": cfg.Title, "deck": b.Deck.Name, "label": labels[b.Deck.Name], "labels": labels,
	}
}

// Page is the presenter's page of slide n.
func (s *Server) Page(n int) (string, bool) {
	s.mu.RLock()
	if h, ok := s.pages[n]; ok {
		s.mu.RUnlock()
		return h, true
	}
	p, b := s.proj, s.built
	s.mu.RUnlock()
	if n < 0 || n >= len(b.Slides) {
		return "", false
	}
	pg := page{
		HTML:        template.HTML(b.Slides[n]),
		Lang:        p.Config.Lang,
		Title:       p.Config.Title,
		Config:      s.runtimeConfig(p, b),
		CurrentStep: n,
		StepCount:   len(b.Slides) - 1,
		DevMode:     s.o.Dev,
	}
	if n > 0 {
		pg.PrevURL = stepURL(n - 1)
	}
	if n < len(b.Slides)-1 {
		pg.NextURL = stepURL(n + 1)
	}
	var sb strings.Builder
	if err := s.tmpl.Execute(&sb, pg); err != nil {
		return "", false
	}
	h := sb.String()
	s.mu.Lock()
	if s.built == b {
		s.pages[n] = h
	}
	s.mu.Unlock()
	return h, true
}

// Index, Catalog and Static feed the audience copy.
func (s *Server) Index() []deck.IndexRow { _, b := s.current(); return b.Index }

// Catalog is the exercise catalog shared by the decks.
func (s *Server) Catalog() []*deck.Exercise { p, _ := s.current(); return p.Catalog }

// Static serves the assets.
func (s *Server) Static() http.Handler { return s.static }

func (s *Server) slide(w http.ResponseWriter, r *http.Request, n int) {
	h, ok := s.Page(n)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, h)
}

func (s *Server) handler(lr *livereload.Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) { s.slide(w, r, 0) })
	mux.HandleFunc("GET /last", func(w http.ResponseWriter, r *http.Request) {
		_, b := s.current()
		http.Redirect(w, r, stepURL(len(b.Slides)-1), http.StatusSeeOther)
	})
	mux.HandleFunc("GET /shell", s.term.Shell)
	mux.HandleFunc("GET /shell/", s.term.Shell)
	mux.HandleFunc("GET /terminal", s.term.Page)
	mux.HandleFunc("/ws/terminal", s.term.WebSocket)
	mux.HandleFunc(terminal.AssetPrefix, s.term.Asset)
	mux.HandleFunc("/ping", ping)
	mux.HandleFunc("GET /__pdf", s.pdf)
	if lr != nil {
		lr.RegisterHandlers(mux)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if n, err := strconv.Atoi(strings.TrimPrefix(path, "/")); err == nil && !strings.HasPrefix(path, "/+") {
			s.slide(w, r, n)
			return
		}
		p, b := s.current()
		switch {
		case path == "/media/search-"+b.Deck.Name+".json":
			writeJSON(w, b.SearchJSON())
			return
		case strings.HasPrefix(path, "/media/search-") && strings.HasSuffix(path, ".json"):
			name := strings.TrimSuffix(strings.TrimPrefix(path, "/media/search-"), ".json")
			if other, ok := p.Decks[name]; ok {
				writeJSON(w, other.SearchJSON())
				return
			}
		case path == "/media/exercises.json":
			writeJSON(w, p.CatalogJSON())
			return
		}
		if s.o.Dev {
			w.Header().Set("Cache-Control", "no-cache")
		}
		s.static.ServeHTTP(w, r)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// ping tells a slide whether a web page it embeds is up (demoit's <web-page>).
func ping(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("url")
	if pu, err := url.Parse(u); err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
		http.Error(w, "Unable to ping", http.StatusBadRequest)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodHead, u, http.NoBody)
	if err != nil {
		http.Error(w, "Unable to ping", http.StatusBadRequest)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
}

func (s *Server) base() string {
	host := s.o.Host
	if host == "localhost" || host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(s.o.Port)))
}

// pdf is "e" in the deck: the range as a PDF file, no print dialog.
func (s *Server) pdf(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, err1 := strconv.Atoi(q.Get("from"))
	b, err2 := strconv.Atoi(q.Get("to"))
	if err1 != nil || err2 != nil {
		http.Error(w, "from and to must be numbers", http.StatusBadRequest)
		return
	}
	if a > b {
		a, b = b, a
	}
	theme := ""
	if q.Get("theme") == "halloween" {
		theme = "halloween"
	}
	mode := pdf.ModeExact
	if q.Get("mode") == pdf.ModeText {
		mode = pdf.ModeText
	}
	data, name, err := s.PDF(r.Context(), a, b, theme, mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	_, _ = w.Write(data)
}

// PDF prints slides a..b of the running deck.
func (s *Server) PDF(ctx context.Context, a, b int, theme, mode string) ([]byte, string, error) {
	s.pdfMu.Lock()
	defer s.pdfMu.Unlock()
	p, built := s.current()
	last := len(built.Slides) - 1
	a, b = max(0, min(a, last)), max(0, min(b, last))
	chrome := pdf.FindChrome()
	if chrome == "" {
		chrome = "no Chrome found"
	}
	fmt.Fprintf(s.o.Out, "PDF (%s): slides %d-%d with %s…\n", mode, a, b, chrome)
	start := time.Now()
	data, err := pdf.Render(ctx, pdf.Request{Base: s.base(), Deck: built.Deck.Name, From: a, To: b, Theme: theme, Mode: mode})
	if err != nil {
		fmt.Fprintf(s.o.Out, "PDF failed: %v\n", err)
		return nil, "", err
	}
	fmt.Fprintf(s.o.Out, "PDF ready: %d KB in %.0f s\n", len(data)/1024, time.Since(start).Seconds())
	return data, pdf.Name(p.Config.Title, pdf.Modules(built.Index), a, b), nil
}

// Run serves until ctx is done.
func (s *Server) Run(ctx context.Context) error {
	p, b := s.current()
	cfg := p.Config
	var lr *livereload.Server
	if s.o.Dev {
		lr = livereload.New(s.o.Port)
		slides := filepath.ToSlash(filepath.Clean(cfg.Slides)) + "/"
		assets := filepath.ToSlash(filepath.Clean(cfg.Assets)) + "/"
		lr.OnChange = func(file string) bool {
			file = filepath.ToSlash(file)
			switch {
			case strings.HasPrefix(file, assets):
				return true
			case strings.HasPrefix(file, slides) || file == "respawn.yaml" || file == "demoit.html":
				start := time.Now()
				if err := s.rebuild(); err != nil {
					fmt.Fprintf(s.o.Out, "rebuild failed: %v\n", err)
					return false
				}
				_, nb := s.current()
				fmt.Fprintf(s.o.Out, "rebuilt %s: %d slides in %d ms\n", nb.Deck.Name, len(nb.Slides), time.Since(start).Milliseconds())
				return true
			}
			return false
		}
		if err := lr.Watch(ctx, s.dir); err != nil {
			return err
		}
	}

	addr := net.JoinHostPort(s.o.Host, strconv.Itoa(s.o.Port))
	srv := &http.Server{Addr: addr, Handler: s.handler(lr), ReadHeaderTimeout: 10 * time.Second}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("presenter port: %w", err)
	}
	errs := make(chan error, 3)
	go func() { errs <- srv.Serve(ln) }()
	label := b.Deck.Label
	if label == "" {
		label = b.Deck.Name
	}
	fmt.Fprintf(s.o.Out, "%s · %s: %d slides on http://%s\n", cfg.Title, label, len(b.Slides), addr)
	if s.o.Dev {
		fmt.Fprintln(s.o.Out, "Live reload on: edit the slides, the browser follows.")
	}

	var tun *tunnel.Tunnel
	if s.o.Audience {
		public := ""
		if s.o.Tunnel {
			if cfg.Tunnel.Host == "" {
				return errors.New("--tunnel needs tunnel.host in respawn.yaml (the public address of the room's copy)")
			}
			public = "https://" + cfg.Tunnel.Host
		}
		aport := s.o.Port + 2
		s.aud = audience.New(s, audience.Options{
			Deck: b.Deck.Name, Port: aport, StateDir: filepath.Join(s.dir, ".respawn"),
			Public: public, Session: s.o.Session, Resume: s.o.Resume, Live: s.o.Live, Title: cfg.Title,
		})
		ln2, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", aport))
		if err != nil {
			return fmt.Errorf("audience port: %w", err)
		}
		asrv := &http.Server{Handler: s.aud, ReadHeaderTimeout: 10 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
		go func() { errs <- asrv.Serve(ln2) }()
		defer asrv.Close()
		if s.o.Tunnel {
			tun, err = tunnel.Start(cfg.Tunnel.Name, cfg.Tunnel.Host, aport)
			if err != nil {
				return err
			}
			defer tun.Stop()
			fmt.Fprintf(s.o.Out, "Cloudflare Tunnel %s -> https://%s (log: %s)\n", cfg.Tunnel.Name, cfg.Tunnel.Host, tun.Log)
			go func() {
				if err := tun.Wait(); err != nil && ctx.Err() == nil {
					fmt.Fprintf(s.o.Out, "The tunnel stopped: %v (log: %s)\n", err, tun.Log)
				}
			}()
		}
		fmt.Fprintf(s.o.Out, "Audience copy: %s  session code %s\n", s.aud.Base(), s.aud.Code())
		fmt.Fprintf(s.o.Out, "Join link: %s   press j in the deck to show its QR code.\n", s.aud.JoinURL())
	}

	select {
	case <-ctx.Done():
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	return nil
}

// ExportPDF serves the deck on a free local port just long enough to print slides a..b
// (to < 0: the last slide). It is `respawn pdf`.
func (s *Server) ExportPDF(ctx context.Context, a, b int, theme, mode string) ([]byte, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	s.o.Host, s.o.Port = "127.0.0.1", ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: s.handler(nil), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	if b < 0 {
		_, built := s.current()
		b = len(built.Slides) - 1
	}
	return s.PDF(ctx, a, b, theme, mode)
}
