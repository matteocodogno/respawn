package deck

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Blocks renders a Markdown body: the rendered blocks joined by newlines.
func Blocks(src string) (string, error) {
	parts, err := blockList(src)
	if err != nil {
		return "", err
	}
	return strings.Join(parts, "\n"), nil
}

var (
	reBullet  = regexp.MustCompile(`^- `)
	reOrdered = regexp.MustCompile(`^\d+\. `)
)

func blockList(src string) ([]string, error) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		// code fence
		if ch, n := isFence(line); n > 0 {
			info := strings.TrimSpace(line[n:])
			j := i + 1
			for j < len(lines) {
				if c2, n2 := isFence(lines[j]); c2 == ch && n2 >= n && strings.Trim(lines[j], ch) == "" {
					break
				}
				j++
			}
			if j >= len(lines) {
				return nil, fmt.Errorf("code block starting %q is not closed", line)
			}
			h, err := fenced(info, strings.Join(lines[i+1:j], "\n"))
			if err != nil {
				return nil, err
			}
			out = append(out, h)
			i = j + 1
			continue
		}
		// component
		if strings.HasPrefix(line, ":::") {
			n := 0
			for n < len(line) && line[n] == ':' {
				n++
			}
			closer := strings.Repeat(":", n)
			head := strings.TrimSpace(line[n:])
			j := i + 1
			for j < len(lines) && strings.TrimRight(lines[j], " \t") != closer {
				j++
			}
			if j >= len(lines) {
				return nil, fmt.Errorf("component %q is not closed by %q", head, closer)
			}
			h, err := component(head, strings.Join(lines[i+1:j], "\n"))
			if err != nil {
				return nil, err
			}
			out = append(out, h)
			i = j + 1
			continue
		}
		// lists
		if reBullet.MatchString(line) || reOrdered.MatchString(line) {
			ordered := reOrdered.MatchString(line)
			var items []Inline
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
				l := lines[i]
				if ordered && reOrdered.MatchString(l) {
					items = append(items, Inline{Text: reOrdered.ReplaceAllString(l, "")})
				} else if !ordered && reBullet.MatchString(l) {
					items = append(items, Inline{Text: l[2:]})
				} else if len(items) > 0 {
					items[len(items)-1].Text += "\n" + strings.TrimLeft(l, " ")
				}
				i++
			}
			out = append(out, list(items, ordered, false))
			continue
		}
		// paragraph
		var para []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			if _, n := isFence(lines[i]); n > 0 || strings.HasPrefix(lines[i], ":::") {
				break
			}
			para = append(para, lines[i])
			i++
		}
		out = append(out, `<p>`+MD(strings.Join(para, "\n"))+`</p>`)
	}
	return out, nil
}

// ---------------------------------------------------------------- params

func parseParams(s string) (string, map[string]string, error) {
	fields := []string{}
	var cur strings.Builder
	inQ := false
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && inQ:
			esc = true
		case r == '"':
			inQ = !inQ
		case r == ' ' && !inQ:
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if inQ {
		return "", nil, fmt.Errorf("unclosed quote in %q", s)
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	params := map[string]string{}
	name := ""
	for i, f := range fields {
		if i == 0 && !strings.Contains(f, "=") {
			name = f
			continue
		}
		if k, v, ok := strings.Cut(f, "="); ok {
			params[k] = v
		} else {
			params[f] = "true"
		}
	}
	return name, params, nil
}

func flag(p map[string]string, k string, def bool) bool {
	v, ok := p[k]
	if !ok {
		return def
	}
	return v != "false" && v != "0" && v != "no"
}

// ---------------------------------------------------------------- fenced code

func fenced(info, code string) (string, error) {
	lang, p, err := parseParams(info)
	if err != nil {
		return "", err
	}
	switch lang {
	case "cmd":
		cls := "cmd"
		if flag(p, "noprompt", false) {
			cls = "cmd noprompt"
		}
		return `<div class="` + cls + `">` + escape(code, true) + `</div>`, nil
	case "mermaid":
		return "<div class=\"mermaid\">\n" + escape(strings.TrimSpace(code), false) + "\n</div>", nil
	}
	if lang == "text" {
		lang = ""
	}
	return codeCard(code, p["title"], p["size"], lang, flag(p, "scroll", false), flag(p, "nolig", false)), nil
}

func codeCard(code, title, size, lang string, scroll, nolig bool) string {
	var out []string
	for _, line := range strings.Split(strings.Trim(code, "\n"), "\n") {
		esc := escape(line, false)
		st := strings.TrimSpace(line)
		if lang != "md" && (strings.HasPrefix(st, "//") || (strings.HasPrefix(st, "#") && (lang == "bash" || lang == "sh"))) {
			esc = `<span class="c">` + esc + `</span>`
		}
		out = append(out, esc)
	}
	bar := `<div class="bar"><i></i><i></i><i></i>` + escape(title, true) + `</div>`
	cls := "code " + size
	if scroll {
		cls += " scroll"
	}
	pre := "<pre>"
	if nolig {
		pre = `<pre style="font-variant-ligatures:none">`
	}
	return `<div class="` + cls + `">` + bar + pre + strings.Join(out, "\n") + `</pre></div>`
}

// ---------------------------------------------------------------- components

func tone(t string) string {
	switch t {
	case "primary", "secondary", "tertiary", "muted":
		return " t-" + t
	}
	return ""
}

func list(items []Inline, ordered, big bool) string {
	tag := "ul"
	if ordered {
		tag = "ol"
	}
	cls := "list"
	if big {
		cls = "list big"
	}
	var b strings.Builder
	for _, it := range items {
		b.WriteString("<li>" + it.Render() + "</li>")
	}
	return "<" + tag + ` class="` + cls + `">` + b.String() + "</" + tag + ">"
}

// textOf reads the content of a text component: plain text, or YAML {text: ...}.
func textOf(content string) (Inline, error) {
	if strings.HasPrefix(content, "text:") {
		var m map[string]any
		if err := yaml.Unmarshal([]byte(content), &m); err != nil {
			return Inline{}, err
		}
		return inline(m["text"]), nil
	}
	return Inline{Text: content}, nil
}

func yamlOf(content string, dst any) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return yaml.Unmarshal([]byte(content), dst)
}

func component(head, content string) (string, error) {
	name, p, err := parseParams(head)
	if err != nil {
		return "", err
	}
	wrap := func(h string, err error) (string, error) {
		if err != nil {
			return "", fmt.Errorf("::: %s: %w", name, err)
		}
		return h, nil
	}
	switch name {
	case "raw":
		return content, nil
	case "p", "lead", "small", "label", "callout", "prompt", "reveal", "stat":
		t, err := textOf(content)
		if err != nil {
			return wrap("", err)
		}
		return wrap(textComponent(name, p, t), nil)
	case "list":
		var items []any
		if err := yamlOf(content, &items); err != nil {
			return wrap("", err)
		}
		var in []Inline
		for _, it := range items {
			in = append(in, inline(it))
		}
		return list(in, flag(p, "ordered", false), flag(p, "big", false)), nil
	case "img":
		var m map[string]any
		if err := yamlOf(content, &m); err != nil {
			return wrap("", err)
		}
		style := ""
		if h := p["height"]; h != "" {
			style = ` style="max-height:calc(var(--u) * ` + h + `)"`
		}
		cap := ""
		if c := inline(m["caption"]); !c.Empty() {
			cap = "<figcaption>" + c.Render() + "</figcaption>"
		}
		return `<figure class="img"><img src="/images/` + p["src"] + `" alt=""` + style + `>` + cap + `</figure>`, nil
	case "nexttoken":
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(content), &node); err != nil {
			return wrap("", err)
		}
		data := yamlToJSONish(&node)
		om := orderedMap{keys: []string{"prompt", "steps", "temperature"}, vals: map[string]any{}}
		if m, ok := data.(orderedMap); ok {
			om.vals["prompt"] = m.vals["prompt"]
			om.vals["steps"] = m.vals["steps"]
		}
		t := 0.7
		if v, ok := p["temperature"]; ok {
			t, _ = strconv.ParseFloat(v, 64)
		}
		om.vals["temperature"] = t
		return `<div class="nexttoken" data-nt="` + escape(pyJSON(om), true) + `"></div>`, nil
	case "card":
		var m map[string]any
		if err := yamlOf(content, &m); err != nil {
			return wrap("", err)
		}
		return wrap(card(m, "primary", false))
	case "cards", "grid", "flow":
		var items []map[string]any
		if err := yamlOf(content, &items); err != nil {
			return wrap("", err)
		}
		def := p["tone"]
		if def == "" {
			def = "primary"
		}
		plain := flag(p, "plain", false)
		var cards []string
		for _, it := range items {
			h, err := card(it, def, plain)
			if err != nil {
				return wrap("", err)
			}
			cards = append(cards, h)
		}
		if name == "flow" {
			var b strings.Builder
			for i, c := range cards {
				if i > 0 {
					b.WriteString(`<span class="arrow">` + Icon("arrow_forward") + `</span>`)
				}
				b.WriteString(c)
			}
			return `<div class="flow">` + b.String() + `</div>`, nil
		}
		cols := len(cards) // Cards: one per card, at most 4; Grid: 3
		if name == "cards" && cols > 4 {
			cols = 4
		}
		if name == "grid" {
			cols = 3
		}
		if c, ok := p["cols"]; ok {
			cols, _ = strconv.Atoi(c)
		}
		g := ""
		if flag(p, "grow", false) {
			g = " grow"
		}
		n := strconv.Itoa(cols)
		return `<div class="grid cols-` + n + g + `" style="--cols:` + n + `">` + strings.Join(cards, "\n") + `</div>`, nil
	case "kv":
		var pairs [][]any
		if err := yamlOf(content, &pairs); err != nil {
			return wrap("", err)
		}
		var b strings.Builder
		for _, kv := range pairs {
			if len(kv) != 2 {
				return wrap("", fmt.Errorf("each kv row needs two values"))
			}
			b.WriteString("<div><dt>" + inline(kv[0]).Render() + "</dt><dd>" + inline(kv[1]).Render() + "</dd></div>")
		}
		return `<dl class="kv">` + b.String() + `</dl>`, nil
	case "steps":
		var items []map[string]any
		if err := yamlOf(content, &items); err != nil {
			return wrap("", err)
		}
		var b strings.Builder
		for _, it := range items {
			inner := ""
			if t := inline(it["title"]); !t.Empty() {
				inner += "<h3>" + t.Render() + "</h3>"
			}
			if t := inline(it["text"]); !t.Empty() {
				inner += `<div class="text">` + t.Render() + `</div>`
			}
			if x, ok := it["extra"]; ok && x != nil {
				h, err := Blocks(fmt.Sprint(x))
				if err != nil {
					return wrap("", err)
				}
				inner += h
			}
			b.WriteString(`<div class="step"><div class="tile num">` + inline(it["num"]).Render() + `</div><div>` + inner + `</div></div>`)
		}
		return `<div class="steps">` + b.String() + `</div>`, nil
	case "table":
		var t struct {
			Headers []any   `yaml:"headers"`
			Rows    [][]any `yaml:"rows"`
		}
		if err := yamlOf(content, &t); err != nil {
			return wrap("", err)
		}
		first := flag(p, "first_col", true)
		th := ""
		for _, h := range t.Headers {
			th += "<th>" + inline(h).Render() + "</th>"
		}
		var trs strings.Builder
		for _, r := range t.Rows {
			trs.WriteString("<tr>")
			for i, c := range r {
				cls := ""
				if first && i == 0 {
					cls = ` class="first"`
				}
				trs.WriteString("<td" + cls + ">" + inline(c).Render() + "</td>")
			}
			trs.WriteString("</tr>")
		}
		head := ""
		if th != "" {
			head = "<thead><tr>" + th + "</tr></thead>"
		}
		cls := "tbl"
		if flag(p, "compact", false) {
			cls = "tbl compact"
		}
		if flag(p, "reveal", false) {
			cls += " reveal-rows"
		}
		return `<table class="` + cls + `">` + head + "<tbody>" + trs.String() + "</tbody></table>", nil
	case "cols":
		var cols []string
		dropped := false
		for _, c := range splitColumns(content) {
			h, err := Blocks(c)
			if err != nil {
				return wrap("", err)
			}
			if strings.TrimSpace(h) != "" {
				cols = append(cols, h)
			} else {
				dropped = true // an empty column: the others share the row evenly
			}
		}
		style := ` style="--tpl:repeat(` + strconv.Itoa(len(cols)) + `, minmax(0,1fr))"`
		if tpl := p["tpl"]; tpl != "" && !dropped {
			style = ` style="--tpl:` + tpl + `"`
		}
		var b strings.Builder
		for _, c := range cols {
			b.WriteString(`<div class="col">` + c + `</div>`)
		}
		g := ""
		if flag(p, "grow", true) {
			g = " grow"
		}
		return `<div class="cols` + g + `"` + style + `>` + b.String() + `</div>`, nil
	case "bars":
		var rows [][]any
		if err := yamlOf(content, &rows); err != nil {
			return wrap("", err)
		}
		mx := 0.0
		if m, ok := p["max"]; ok {
			mx, _ = strconv.ParseFloat(m, 64)
		}
		if mx == 0 {
			for _, r := range rows {
				mx = math.Max(mx, num(r[1]))
			}
		}
		var b strings.Builder
		for _, r := range rows {
			w := 100 * num(r[1]) / mx
			b.WriteString(`<div class="barrow"><span>` + inline(r[0]).Render() + `</span><div><div class="bar" style="width:` + fixed(w, 1) +
				`%"></div></div><span class="pct">` + inline(r[2]).Render() + `</span></div>`)
		}
		return `<div class="bars">` + b.String() + `</div>`, nil
	case "splash":
		var m map[string]any
		if err := yamlOf(content, &m); err != nil {
			return wrap("", err)
		}
		var chips []any
		if c, ok := m["chips"].([]any); ok {
			chips = c
		}
		return splash(p["icon"], p["word"], inline(m["caption"]), chips, p["tone"]), nil
	}
	return "", fmt.Errorf("unknown component ::: %s", name)
}

func num(v any) float64 {
	switch t := v.(type) {
	case int:
		return float64(t)
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

// splitColumns splits a cols container on "+++" lines (outside code fences).
func splitColumns(content string) []string {
	lines := strings.Split(content, "\n")
	var cols []string
	var cur []string
	fenceCh, fenceN := "", 0
	for _, l := range lines {
		if fenceN > 0 {
			if ch, n := isFence(l); ch == fenceCh && n >= fenceN && strings.Trim(l, ch) == "" {
				fenceCh, fenceN = "", 0
			}
		} else if ch, n := isFence(l); n > 0 {
			fenceCh, fenceN = ch, n
		} else if l == "+++" {
			cols = append(cols, strings.Join(cur, "\n"))
			cur = nil
			continue
		}
		cur = append(cur, l)
	}
	return append(cols, strings.Join(cur, "\n"))
}

func textComponent(name string, p map[string]string, t Inline) string {
	switch name {
	case "p":
		if c := p["class"]; c != "" {
			return `<p class="` + c + `">` + t.Render() + `</p>`
		}
		return `<p>` + t.Render() + `</p>`
	case "lead", "small":
		return `<p class="` + name + `">` + t.Render() + `</p>`
	case "label":
		tn := p["tone"]
		if tn == "" {
			tn = "primary"
		}
		return `<div class="label ` + tn + `">` + t.Render() + `</div>`
	case "callout":
		ic := p["icon"]
		if ic == "" {
			ic = "lightbulb"
		}
		tn := p["tone"]
		if tn == "" {
			tn = "primary"
		}
		return `<div class="callout` + tone(tn) + `">` + Icon(ic) + `<div>` + t.Render() + `</div></div>`
	case "prompt":
		l := p["label"]
		if l == "" {
			l = "PROMPT"
		}
		return `<div class="prompt"><span class="label">` + MD(l) + `</span>` + t.Render() + `</div>`
	case "reveal":
		l := p["label"]
		if l == "" {
			l = "Reveal"
		}
		return `<div class="reveal"><button class="reveal-btn" type="button">` + Icon("visibility") + ` ` + MD(l) +
			`</button><div class="reveal-note">` + t.Render() + `</div></div>`
	case "stat":
		tn := p["tone"]
		if tn == "" {
			tn = "primary"
		}
		return `<div class="stat` + tone(tn) + `"><span class="big">` + MD(p["big"]) + `</span><span class="lead">` + t.Render() + `</span></div>`
	}
	return ""
}

func card(m map[string]any, defTone string, defPlain bool) (string, error) {
	tn := defTone
	if v, ok := m["tone"]; ok && v != nil {
		tn = fmt.Sprint(v)
	}
	plain := defPlain
	if v, ok := m["plain"].(bool); ok {
		plain = v
	}
	hl, _ := m["hl"].(bool)
	reveal := inline(m["reveal"])
	cls := "card" + tone(tn)
	if hl {
		cls += " hl"
	}
	if plain {
		cls += " plain"
	}
	if !reveal.Empty() {
		cls += " has-reveal"
	}
	var parts []string
	if v := inline(m["icon"]); !v.Empty() {
		ic := Icon(v.Text)
		if v.HTML {
			ic = `<span class="ms">` + v.Text + `</span>`
		}
		parts = append(parts, `<div class="tile">`+ic+`</div>`)
	}
	if v, ok := m["num"]; ok && v != nil {
		parts = append(parts, `<div class="tile num">`+inline(v).Render()+`</div>`)
	}
	if v := inline(m["big"]); !v.Empty() {
		parts = append(parts, `<div class="big">`+v.Render()+`</div>`)
	}
	if v := inline(m["label"]); !v.Empty() {
		parts = append(parts, `<div class="label">`+v.Render()+`</div>`)
	}
	if v := inline(m["title"]); !v.Empty() {
		parts = append(parts, "<h3>"+v.Render()+"</h3>")
	}
	if v := inline(m["text"]); !v.Empty() {
		parts = append(parts, `<div class="text">`+v.Render()+`</div>`)
	}
	if v, ok := m["body"]; ok && v != nil {
		h, err := Blocks(fmt.Sprint(v))
		if err != nil {
			return "", err
		}
		if h != "" {
			parts = append(parts, h)
		}
	}
	if !reveal.Empty() {
		parts = append(parts, `<span class="reveal-stamp">`+Icon("add")+`</span><div class="reveal-note">`+reveal.Render()+`</div>`)
	}
	return `<div class="` + cls + `">` + strings.Join(parts, "") + `</div>`, nil
}

func splash(iconName, word string, caption Inline, chips []any, tn string) string {
	if tn == "" {
		tn = "primary"
	}
	var pts []string
	for i := 0; i < 28; i++ {
		a := float64(i) / 28 * 2 * math.Pi
		r := 38.0
		if i%2 == 0 {
			r = 48
		} else if i%4 == 1 {
			r = 35
		}
		pts = append(pts, fixed(50+math.Cos(a)*r, 1)+","+fixed(50+math.Sin(a)*r, 1))
	}
	burst := `<svg class="sp-burst" viewBox="0 0 100 100" aria-hidden="true"><polygon points="` + strings.Join(pts, " ") + `"/></svg>`
	cap := ""
	if !caption.Empty() {
		cap = `<p class="sp-cap">` + caption.Render() + `</p>`
	}
	ch := ""
	if len(chips) > 0 {
		ch = `<div class="sp-chips">`
		for _, c := range chips {
			ch += "<span>" + inline(c).Render() + "</span>"
		}
		ch += "</div>"
	}
	return `<div class="splash` + tone(tn) + `"><div class="sp-art">` + burst + Icon(iconName) +
		`<span class="sp-word">` + MD(word) + `</span></div><div class="sp-text">` + cap + ch + `</div></div>`
}

// yamlToJSONish converts a YAML node keeping int/float and key order (for nexttoken data).
func yamlToJSONish(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		return yamlToJSONish(n.Content[0])
	case yaml.MappingNode:
		om := orderedMap{vals: map[string]any{}}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			om.keys = append(om.keys, k)
			om.vals[k] = yamlToJSONish(n.Content[i+1])
		}
		return om
	case yaml.SequenceNode:
		var out []any
		for _, c := range n.Content {
			out = append(out, yamlToJSONish(c))
		}
		return out
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!int":
			v, _ := strconv.Atoi(n.Value)
			return v
		case "!!float":
			v, _ := strconv.ParseFloat(n.Value, 64)
			return v
		case "!!bool":
			return n.Value == "true"
		case "!!null":
			return nil
		}
		return n.Value
	}
	return nil
}
