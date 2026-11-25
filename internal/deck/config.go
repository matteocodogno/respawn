package deck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// ConfigFile is the name of a presentation's configuration file.
const ConfigFile = "respawn.yaml"

// Deck is one deck built from the presentation's slides (a presentation can hold several,
// e.g. a short and a long version of the same course).
type Deck struct {
	Name  string `yaml:"name"`
	Label string `yaml:"label"`
	Port  int    `yaml:"port"`
}

// Config is respawn.yaml.
type Config struct {
	Respawn string `yaml:"respawn"` // version constraint, e.g. ">=0.1, <1.0"
	Title   string `yaml:"title"`
	Brand   string `yaml:"brand"`  // footer brand, e.g. "AI4DEV // FIELD MANUAL"
	Slides  string `yaml:"slides"` // folder of .md files, read in name order
	Assets  string `yaml:"assets"` // folder overriding the built-in assets (images/, media/, js/, fonts/, style.css)
	Lang    string `yaml:"lang"`
	// Namespace prefixes what the deck keeps in the browser (sticky notes, timers...).
	Namespace string `yaml:"namespace"`
	// Theme is a stylesheet loaded over the built-in look: a theme of respawn
	// (assets/themes/<name>.css) or of the presentation (its assets/themes/<name>.css).
	Theme string `yaml:"theme"`
	// Quiz holds the rules of the live quiz ("::: quiz" on the slides).
	Quiz    QuizConfig `yaml:"quiz"`
	Decks   []Deck     `yaml:"decks"`
	Accents []string   `yaml:"accents"`

	Labs struct {
		Kickers string `yaml:"kickers"`  // slides with a live terminal
		HandsOn string `yaml:"hands_on"` // ... and these, when they show a command
		Folder  string `yaml:"folder"`   // working folder of the terminals
	} `yaml:"labs"`

	Catalog struct {
		Kickers string `yaml:"kickers"` // slides that go to the exercise catalog
		Primary string `yaml:"primary"` // deck whose version wins when an exercise is in several decks
	} `yaml:"catalog"`

	Footer struct {
		Intro string `yaml:"intro"` // footer chip before the first module
	} `yaml:"footer"`

	Cover struct {
		Scribbles []string `yaml:"scribbles"`
		Press     string   `yaml:"press"`
	} `yaml:"cover"`

	History []string          `yaml:"history"` // extra commands for the terminals' shell history
	Gif     string            `yaml:"gif"`     // "h": a gif that pops in from a corner (path under assets)
	Strings map[string]string `yaml:"strings"` // runtime text overrides (see assets/js/respawn.js)
	Tunnel  struct {
		Name string `yaml:"name"`
		Host string `yaml:"host"`
	} `yaml:"tunnel"`

	Dir string `yaml:"-"` // folder of respawn.yaml

	reLab, reHandsOn, reCatalog *regexp.Regexp
}

// Defaults applied to every presentation.
func (c *Config) defaults() {
	if c.Slides == "" {
		c.Slides = "slides"
	}
	if c.Assets == "" {
		c.Assets = "assets"
	}
	if c.Brand == "" {
		c.Brand = "RESPAWN // FIELD MANUAL"
	}
	c.Quiz.defaults()
	if c.Namespace == "" {
		c.Namespace = "respawn"
	}
	if c.Lang == "" {
		c.Lang = "en"
	}
	if len(c.Decks) == 0 {
		c.Decks = []Deck{{Name: "main", Label: "", Port: 8888}}
	}
	for i := range c.Decks {
		if c.Decks[i].Port == 0 {
			// pairs of decks, ten ports apart: each deck's audience copy takes port+2
			c.Decks[i].Port = 8888 + (i/2)*10 + i%2
		}
	}
	if len(c.Accents) == 0 {
		c.Accents = []string{"yellow", "cyan", "magenta", "green", "orange", "purple", "blue", "pink", "yellow", "cyan"}
	}
	if c.Labs.Kickers == "" {
		c.Labs.Kickers = `^\s*(lab|demo)`
	}
	if c.Labs.HandsOn == "" {
		c.Labs.HandsOn = `^\s*(exercise)`
	}
	if c.Labs.Folder == "" {
		c.Labs.Folder = "workspace"
	}
	if c.Catalog.Kickers == "" {
		c.Catalog.Kickers = `^\s*(recap|recall|conclusion)`
	}
	if c.Catalog.Primary == "" {
		c.Catalog.Primary = c.Decks[len(c.Decks)-1].Name
	}
	if c.Footer.Intro == "" {
		c.Footer.Intro = "INTRO"
	}
	if c.Cover.Press == "" {
		c.Cover.Press = "PRESS START →"
	}
	if c.Tunnel.Name == "" {
		c.Tunnel.Name = "respawn"
	}
}

// LoadConfig reads respawn.yaml from dir.
func LoadConfig(dir string) (*Config, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(abs, ConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no %s in %s: run respawn new, or cd into a presentation", ConfigFile, abs)
	}
	if err != nil {
		return nil, err
	}
	return ParseConfig(data, abs)
}

// ParseConfig parses respawn.yaml content.
func ParseConfig(data []byte, dir string) (*Config, error) {
	c := &Config{}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	c.Dir = dir
	c.defaults()
	for _, p := range []struct {
		src string
		dst **regexp.Regexp
	}{{c.Labs.Kickers, &c.reLab}, {c.Labs.HandsOn, &c.reHandsOn}, {c.Catalog.Kickers, &c.reCatalog}} {
		re, err := regexp.Compile("(?i)" + p.src)
		if err != nil {
			return nil, fmt.Errorf("%s: bad pattern %q: %w", ConfigFile, p.src, err)
		}
		*p.dst = re
	}
	seen := map[string]bool{}
	for _, d := range c.Decks {
		if d.Name == "" || seen[d.Name] {
			return nil, fmt.Errorf("%s: every deck needs a distinct name", ConfigFile)
		}
		seen[d.Name] = true
	}
	return c, nil
}

// DeckByName returns the deck with that name (the first one when name is empty).
func (c *Config) DeckByName(name string) (Deck, error) {
	if name == "" {
		return c.Decks[0], nil
	}
	for _, d := range c.Decks {
		if d.Name == name {
			return d, nil
		}
	}
	names := []string{}
	for _, d := range c.Decks {
		names = append(names, d.Name)
	}
	return Deck{}, fmt.Errorf("no deck %q (decks: %v)", name, names)
}

// Path resolves a path relative to the presentation.
func (c *Config) Path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.Dir, p)
}
