package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Project is a loaded presentation: its configuration and every deck, built.
type Project struct {
	Config   *Config
	Decks    map[string]*Built
	Catalog  []*Exercise
	History  []string
	Warnings []string
}

// Load reads and builds a presentation.
func Load(dir string) (*Project, error) {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return nil, err
	}
	return LoadWith(cfg)
}

// LoadWith builds a presentation from an already parsed configuration.
func LoadWith(cfg *Config) (*Project, error) {
	sources, err := ReadSources(cfg.Path(cfg.Slides))
	if err != nil {
		return nil, err
	}
	p := &Project{Config: cfg, Decks: map[string]*Built{}}
	var all []*Built
	for _, d := range cfg.Decks {
		b, err := Build(cfg, sources, d)
		if err != nil {
			return nil, err
		}
		p.Decks[d.Name] = b
		all = append(all, b)
	}
	p.Catalog = SharedCatalog(cfg, p.Decks)
	p.History = History(cfg, all)
	return p, nil
}

// SearchJSON is the search index of one deck.
func (b *Built) SearchJSON() []byte {
	data, _ := json.Marshal(b.Index)
	return data
}

// CatalogJSON is the shared exercise catalog.
func (p *Project) CatalogJSON() []byte {
	if p.Catalog == nil {
		return []byte("[]")
	}
	data, _ := json.Marshal(p.Catalog)
	return data
}

// WriteDebug writes what demoit used to read from disk: demoit.html per deck, the search
// indexes, the catalog and the shell history (respawn build --out).
func (p *Project) WriteDebug(out string) error {
	for name, b := range p.Decks {
		dir := filepath.Join(out, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "demoit.html"), []byte(b.DemoitHTML()), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "search-"+name+".json"), b.SearchJSON(), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(out, "exercises.json"), p.CatalogJSON(), 0o644); err != nil {
		return err
	}
	hist := ""
	for _, c := range p.History {
		hist += c + "\n"
	}
	return os.WriteFile(filepath.Join(out, "history"), []byte(hist), 0o644)
}
