package server

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/matteocodogno/respawn/internal/deck"
)

// DefaultTheme is the built-in look (style.css alone).
const DefaultTheme = "borderlands"

var reThemeName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,40}$`)

// themeName is the theme to load over style.css, "" for the default look.
func themeName(cfg *deck.Config) string {
	t := strings.ToLower(strings.TrimSpace(cfg.Theme))
	if t == "" || t == DefaultTheme {
		return ""
	}
	return t
}

// Themes lists the themes a presentation can use: respawn's and its own assets/themes/*.css.
func Themes(cfg *deck.Config) []string {
	o := newOverlay(cfg.Path(cfg.Assets))
	seen := map[string]bool{DefaultTheme: true}
	for _, f := range []fs.FS{o.embed, o.local} {
		if f == nil {
			continue
		}
		entries, _ := fs.ReadDir(f, "themes")
		for _, e := range entries {
			if name, ok := strings.CutSuffix(e.Name(), ".css"); ok && !e.IsDir() {
				seen[name] = true
			}
		}
	}
	var out []string
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// CheckTheme tells whether the theme of respawn.yaml exists.
func CheckTheme(cfg *deck.Config) error {
	t := themeName(cfg)
	if t == "" {
		return nil
	}
	if !reThemeName.MatchString(t) {
		return fmt.Errorf("%s: theme %q: use a name like clean or my-brand", deck.ConfigFile, cfg.Theme)
	}
	f, err := newOverlay(cfg.Path(cfg.Assets)).Open("themes/" + t + ".css")
	if err != nil {
		return fmt.Errorf("%s: no theme %q (themes: %s; or add %s/themes/%s.css)",
			deck.ConfigFile, t, strings.Join(Themes(cfg), ", "), cfg.Assets, t)
	}
	f.Close()
	return nil
}
