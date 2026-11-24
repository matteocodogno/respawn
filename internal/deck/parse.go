package deck

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Source is one slide as written: its YAML header and its Markdown body.
type Source struct {
	Header map[string]any
	Body   string
	File   string
	Line   int
}

// Slide is one slide resolved for one deck.
type Slide struct {
	Title    Inline
	Kicker   Inline
	Layout   string
	Decks    []string
	Sid      string
	Timer    Inline
	Sub      Inline
	Lead     Inline
	Num      Inline
	Center   bool
	Term     any // nil = automatic, bool, or a folder
	Catalog  *bool
	Exercise string
	Generic  bool
	Notes    string
	Body     string

	src *Source
}

// ReadSources reads every .md file of the slides folder, in name order.
func ReadSources(dir string) ([]*Source, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("slides folder: %w", err)
	}
	names := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []*Source
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		s, err := ParseSlides(string(data), n)
		if err != nil {
			return nil, err
		}
		out = append(out, s...)
	}
	return out, nil
}

func isFence(line string) (string, int) {
	t := strings.TrimLeft(line, " ")
	if t != line {
		return "", 0
	}
	for _, ch := range []string{"`", "~"} {
		n := 0
		for n < len(t) && string(t[n]) == ch {
			n++
		}
		if n >= 3 {
			return ch, n
		}
	}
	return "", 0
}

// ParseSlides splits one file into slides. A slide is
//
//	---
//	yaml header
//	---
//	markdown body
//
// and the body runs until the next line that is exactly "---" (outside code fences).
func ParseSlides(text, file string) ([]*Source, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []*Source
	i := 0
	// anything before the first separator must be blank
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	for i < len(lines) {
		if lines[i] != "---" {
			return nil, fmt.Errorf("%s:%d: expected '---' to start a slide", file, i+1)
		}
		start := i + 1
		j := start
		for j < len(lines) && lines[j] != "---" {
			j++
		}
		if j >= len(lines) {
			return nil, fmt.Errorf("%s:%d: slide header is not closed by '---'", file, i+1)
		}
		header := map[string]any{}
		if err := yaml.Unmarshal([]byte(strings.Join(lines[start:j], "\n")), &header); err != nil {
			return nil, fmt.Errorf("%s:%d: slide header: %w", file, i+1, err)
		}
		if header == nil {
			header = map[string]any{}
		}
		k := j + 1
		fenceCh, fenceN := "", 0
		for k < len(lines) {
			l := lines[k]
			if fenceN > 0 {
				if ch, n := isFence(l); ch == fenceCh && n >= fenceN && strings.Trim(l, ch) == "" {
					fenceCh, fenceN = "", 0
				}
			} else if ch, n := isFence(l); n > 0 {
				fenceCh, fenceN = ch, n
			} else if l == "---" {
				break
			}
			k++
		}
		body := strings.Trim(strings.Join(lines[j+1:k], "\n"), "\n")
		out = append(out, &Source{Header: header, Body: body, File: file, Line: i + 1})
		i = k
	}
	return out, nil
}

// inline reads a header or YAML value as inline text.
func inline(v any) Inline {
	switch t := v.(type) {
	case nil:
		return Inline{}
	case string:
		return Inline{Text: t}
	case map[string]any:
		if r, ok := t["raw"]; ok {
			return Inline{Text: fmt.Sprint(r), HTML: true}
		}
		if r, ok := t["html"]; ok {
			return Inline{Text: fmt.Sprint(r), HTML: true}
		}
	case bool:
		if t {
			return Inline{Text: "True"}
		}
		return Inline{Text: "False"}
	}
	return Inline{Text: pyStr(v)}
}

func (i Inline) Empty() bool { return i.Text == "" }

// Resolve returns the slide as it appears in one deck, or nil when it is not part of it.
func (s *Source) Resolve(deck string) (*Slide, error) {
	h := map[string]any{}
	for k, v := range s.Header {
		h[k] = v
	}
	if ds, ok := h["decks"]; ok && ds != nil {
		in := false
		list, ok := ds.([]any)
		if !ok {
			return nil, fmt.Errorf("%s:%d: decks must be a list", s.File, s.Line)
		}
		for _, d := range list {
			if fmt.Sprint(d) == deck {
				in = true
			}
		}
		if !in {
			return nil, nil
		}
	}
	body := s.Body
	if vs, ok := h["variants"].(map[string]any); ok {
		if v, ok := vs[deck].(map[string]any); ok {
			for k, val := range v {
				if k == "body" {
					body = strings.Trim(fmt.Sprint(val), "\n")
					continue
				}
				h[k] = val
			}
		}
	}
	sl := &Slide{
		Title: inline(h["title"]), Kicker: inline(h["kicker"]), Timer: inline(h["timer"]), Sub: inline(h["sub"]),
		Lead: inline(h["lead"]), Num: inline(h["num"]), Layout: "content", Body: body, src: s,
	}
	if l, ok := h["layout"].(string); ok && l != "" {
		sl.Layout = l
	}
	if v, ok := h["sid"]; ok && v != nil {
		sl.Sid = fmt.Sprint(v)
	}
	if v, ok := h["center"].(bool); ok {
		sl.Center = v
	}
	if v, ok := h["generic"].(bool); ok {
		sl.Generic = v
	}
	if v, ok := h["catalog"].(bool); ok {
		sl.Catalog = &v
	}
	if v, ok := h["exercise"]; ok && v != nil {
		sl.Exercise = fmt.Sprint(v)
	}
	if v, ok := h["notes"]; ok && v != nil {
		sl.Notes = fmt.Sprint(v)
	}
	if v, ok := h["term"]; ok && v != nil {
		sl.Term = v
	}
	switch sl.Layout {
	case "content", "module", "part", "statement", "cover":
	default:
		return nil, fmt.Errorf("%s:%d: unknown layout %q", s.File, s.Line, sl.Layout)
	}
	return sl, nil
}
