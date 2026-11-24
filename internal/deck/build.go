package deck

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

func itoa(i int) string { return strconv.Itoa(i) }

// isPySpace is Python's str.isspace() for one character.
func isPySpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

func pyStrip(s string) string { return strings.TrimFunc(s, isPySpace) }

const pySpace = `[\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`

var (
	reScript    = regexp.MustCompile(`(?s)<script\b.*?</script>`)
	reStyle     = regexp.MustCompile(`(?s)<style\b.*?</style>`)
	reBreaks    = regexp.MustCompile(`<br` + pySpace + `*/?>|</(p|li|div|h1|h2|h3|tr|pre)>`)
	reTag       = regexp.MustCompile(`<[^>]+>`)
	reNL        = regexp.MustCompile(pySpace + `*\n` + pySpace + `*`)
	reBlanks    = regexp.MustCompile(`[ \t]+`)
	reNotes     = regexp.MustCompile(`(?s)<speaker-notes>(.*?)</speaker-notes>`)
	reNotesAll  = regexp.MustCompile(`(?s)\s*<speaker-notes>.*?</speaker-notes>`)
	reNotesBare = regexp.MustCompile(`(?s)<speaker-notes>.*?</speaker-notes>`)
	reFoot      = regexp.MustCompile(`(?s)<div class="foot">.*?</div></section>`)
	reTermLabel = regexp.MustCompile(`<span class="term-label">.*?</span>`)
	reSlug      = regexp.MustCompile(`[^a-z0-9]+`)
	reCmd       = regexp.MustCompile(`(?s)<div class="cmd[^"]*">(.*?)</div>`)
)

// Text turns an HTML fragment into plain text (search index, catalog titles).
func Text(fragment string) string {
	t := reScript.ReplaceAllString(fragment, " ")
	t = reStyle.ReplaceAllString(t, " ")
	t = reBreaks.ReplaceAllString(t, "\n")
	t = html.UnescapeString(reTag.ReplaceAllString(t, " "))
	t = reNL.ReplaceAllString(t, "\n")
	t = reBlanks.ReplaceAllString(t, " ")
	return pyStrip(t)
}

func slug(s string) string {
	return strings.Trim(reSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// IndexRow is one entry of the search index ("/" in the deck).
type IndexRow struct {
	N int    `json:"n"`
	K string `json:"k"`
	T string `json:"t"`
	X string `json:"x"`
	O string `json:"o"`
	M int    `json:"m,omitempty"`
}

// Exercise is one entry of the exercise catalog ("x" in the deck).
type Exercise struct {
	ID      string   `json:"id"`
	Generic bool     `json:"generic"`
	Label   string   `json:"label"`
	Title   string   `json:"title"`
	Timer   string   `json:"timer"`
	Module  string   `json:"module"`
	Steps   []string `json:"steps"`
	Notes   []string `json:"notes"`
	Decks   []string `json:"decks,omitempty"`

	key  string
	next int
}

// Built is one deck, ready to serve.
type Built struct {
	Deck     Deck
	Slides   []string // HTML of each slide; index 0 is the cover
	Index    []IndexRow
	Catalog  []*Exercise
	Terms    []int
	Dropped  []int
	Commands []string
}

func titleOrNum(s *Slide) Inline {
	if !s.Title.Empty() {
		return s.Title
	}
	return s.Num
}

func (b *Built) indexRow(i int, s *Slide, rendered string) IndexRow {
	notes := reNotes.FindStringSubmatch(rendered)
	body := reNotesBare.ReplaceAllString(rendered, "")
	body = reFoot.ReplaceAllString(body, "</section>")
	body = reTermLabel.ReplaceAllString(body, "")
	title := Text(titleOrNum(s).Render())
	kicker := Text(s.Kicker.Render())
	var lines []string
	for _, l := range strings.Split(Text(body), "\n") {
		if pyStrip(l) != "" {
			lines = append(lines, l)
		}
	}
	for _, head := range []string{kicker, title} {
		if len(lines) > 0 && head != "" && pyStrip(lines[0]) == pyStrip(head) {
			lines = lines[1:]
		}
	}
	row := IndexRow{N: i, K: strings.TrimRight(kicker, "_"), T: title, X: strings.Join(lines, " · ")}
	if notes != nil {
		row.O = Text(notes[1])
	}
	if s.Layout == "module" {
		row.M = 1
	}
	return row
}

func (s *Slide) empty() (bool, error) {
	if s.Layout != "content" || !s.Lead.Empty() {
		return false, nil
	}
	b, err := Blocks(s.Body)
	return strings.TrimSpace(b) == "", err
}

func wrapErr(s *Slide, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s:%d: %w", s.src.File, s.src.Line, err)
}

// Build renders one deck of the presentation.
func Build(cfg *Config, sources []*Source, deck Deck) (*Built, error) {
	b := &Built{Deck: deck}
	type kept struct {
		s    *Slide
		term string
	}
	var keep []kept
	module, moduleNo, moduleName := 0, "", ""
	for orig, src := range sources {
		s, err := src.Resolve(deck.Name)
		if err != nil {
			return nil, err
		}
		if s == nil {
			continue
		}
		if s.Layout == "module" {
			module++
			moduleNo = reNonDigit.ReplaceAllString(s.Num.Text, "")
			moduleName = Text(s.Title.Render())
			if moduleNo != "" {
				moduleName = "Module " + moduleNo + " · " + moduleName
			}
		}
		inCatalog := s.Layout == "content" && cfg.reCatalog.MatchString(s.Kicker.Text)
		if s.Catalog != nil {
			inCatalog = *s.Catalog
		}
		if inCatalog {
			// The catalog says what an exercise is for, not when it ran in the deck:
			// "Module 05_" when it was written for that module, "Generic_" when it fits any.
			generic := s.Generic || moduleNo == ""
			cs := *s
			if generic {
				cs.Kicker = Inline{Text: "Generic_"}
			} else {
				cs.Kicker = Inline{Text: "Module " + moduleNo + "_"}
			}
			accent := cfg.Accents[module%len(cfg.Accents)]
			h, err := cs.render(renderCtx{cfg: cfg, accent: accent}, 0, 1, "")
			if err != nil {
				return nil, wrapErr(s, err)
			}
			h = strings.Replace(h, `<section class="slide`, `<section class="slide ex`, 1)
			notes := reNotes.FindStringSubmatch(h)
			h = reNotesAll.ReplaceAllString(h, "")
			key := s.Exercise
			if key == "" {
				key = s.Title.Text
			}
			var ex *Exercise
			if n := len(b.Catalog); n > 0 && b.Catalog[n-1].key == key && b.Catalog[n-1].next == orig {
				ex = b.Catalog[n-1]
			} else {
				id := slug(key)
				if id == "" {
					id = "ex-" + itoa(orig)
				}
				mod := moduleName
				if generic {
					mod = ""
				}
				ex = &Exercise{key: key, ID: id, Generic: generic, Label: strings.ToUpper(strings.TrimRight(cs.Kicker.Text, "_")),
					Title: Text(s.Title.Render()), Timer: s.Timer.Text, Module: mod, Steps: []string{}, Notes: []string{}}
				b.Catalog = append(b.Catalog, ex)
			}
			ex.Steps = append(ex.Steps, h)
			note := ""
			if notes != nil {
				note = Text(notes[1])
			}
			ex.Notes = append(ex.Notes, note)
			if ex.Timer == "" && !s.Timer.Empty() {
				ex.Timer = s.Timer.Text
			}
			ex.next = orig + 1
			continue
		}
		term := ""
		switch t := s.Term.(type) {
		case nil:
			probe, err := s.render(renderCtx{cfg: cfg, accent: "yellow"}, 1, 1, "")
			if err != nil {
				return nil, wrapErr(s, err)
			}
			if cfg.reLab.MatchString(s.Kicker.Text) || (cfg.reHandsOn.MatchString(s.Kicker.Text) && strings.Contains(probe, `class="cmd`)) {
				term = cfg.Labs.Folder
			}
		case bool:
			if t {
				term = cfg.Labs.Folder
			}
		case string:
			term = t
		}
		empty, err := s.empty()
		if err != nil {
			return nil, wrapErr(s, err)
		}
		if empty && term == "" {
			b.Dropped = append(b.Dropped, orig)
			continue
		}
		keep = append(keep, kept{s, term})
	}

	total := len(keep)
	seen := map[string]int{}
	module = 0
	label := cfg.Footer.Intro
	for i, k := range keep {
		s := k.s
		if s.Layout == "module" {
			module++
			digits := reNonDigit.ReplaceAllString(s.Num.Text, "")
			name := Text(s.Title.Render())
			if digits != "" {
				label = strings.ToUpper("MODULE " + digits + " · " + name)
			} else {
				label = strings.ToUpper(name)
			}
		}
		accent := cfg.Accents[module%len(cfg.Accents)]
		if k.term != "" {
			b.Terms = append(b.Terms, i)
		}
		h, err := s.render(renderCtx{cfg: cfg, label: label, accent: accent}, i, total-1, k.term)
		if err != nil {
			return nil, wrapErr(s, err)
		}
		b.Index = append(b.Index, b.indexRow(i, s, h))
		h = strings.Replace(h, `<section class="slide`, `<section data-track="`+deck.Name+`" data-sid="`+sid(s, seen)+`" class="slide`, 1)
		b.Slides = append(b.Slides, h)
	}
	for _, h := range b.Slides {
		for _, m := range reCmd.FindAllStringSubmatch(h, -1) {
			c := pyStrip(html.UnescapeString(m[1]))
			if c != "" && !strings.HasPrefix(c, "/") && !contains(b.Commands, c) {
				b.Commands = append(b.Commands, c)
			}
		}
	}
	return b, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sid(s *Slide, seen map[string]int) string {
	base := slug(s.Kicker.Text + " " + titleOrNum(s).Text)
	if s.Sid != "" {
		base = s.Sid
	}
	if base == "" {
		base = "slide"
	}
	seen[base]++
	if seen[base] == 1 {
		return base
	}
	return base + "-" + itoa(seen[base])
}

// DemoitHTML joins the slides the way demoit's demoit.html does (debugging, golden tests).
func (b *Built) DemoitHTML() string {
	parts := make([]string, len(b.Slides))
	for i, h := range b.Slides {
		parts[i] = "\n<!-- slide " + itoa(i) + " -->\n" + strings.ReplaceAll(h, "---", "&#45;&#45;&#45;") + "\n"
	}
	return strings.Join(parts, "---")
}

// SharedCatalog merges the decks' catalogs: one catalog whatever deck is on. When an
// exercise is in several decks the primary deck's version wins; exercises of other decks
// are slotted in after the one they follow in their own deck.
func SharedCatalog(cfg *Config, built map[string]*Built) []*Exercise {
	order := []string{}
	for _, d := range cfg.Decks {
		if built[d.Name] != nil {
			order = append(order, d.Name)
		}
	}
	if len(order) == 0 {
		return nil
	}
	primary := cfg.Catalog.Primary
	if built[primary] == nil {
		primary = order[len(order)-1]
	}
	ids := map[string]map[string]bool{}
	for _, d := range order {
		ids[d] = map[string]bool{}
		for _, e := range built[d].Catalog {
			ids[d][e.ID] = true
		}
	}
	decksOf := func(id string) []string {
		var out []string
		for _, d := range order {
			if ids[d][id] {
				out = append(out, d)
			}
		}
		return out
	}
	var out []*Exercise
	have := map[string]bool{}
	for _, e := range built[primary].Catalog {
		c := *e
		c.Decks = decksOf(e.ID)
		out = append(out, &c)
		have[e.ID] = true
	}
	for _, d := range order {
		if d == primary {
			continue
		}
		prev := ""
		for _, e := range built[d].Catalog {
			if !have[e.ID] {
				at := len(out)
				if prev == "" {
					at = 0
				} else {
					for i, x := range out {
						if x.ID == prev {
							at = i + 1
							break
						}
					}
				}
				c := *e
				c.Decks = decksOf(e.ID)
				out = append(out[:at], append([]*Exercise{&c}, out[at:]...)...)
				have[e.ID] = true
			}
			prev = e.ID
		}
	}
	return out
}

// History is every command shown on a slide, in deck order, for the terminals' shell history.
func History(cfg *Config, built []*Built) []string {
	var out []string
	for _, b := range built {
		for _, c := range b.Commands {
			if !contains(out, c) {
				out = append(out, c)
			}
		}
	}
	for _, c := range cfg.History {
		if !contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}
