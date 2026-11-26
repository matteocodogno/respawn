package deck

import (
	"regexp"
	"strings"
)

var reNonDigit = regexp.MustCompile(`\D`)

func termDock(folder string) string {
	return `<div class="term-dock"><span class="term-label">LIVE TERMINAL · t</span>` +
		`<web-term path="` + escape(folder, true) + `"></web-term></div>`
}

func zfill2(s string) string {
	for runeLen(s) < 2 {
		s = "0" + s
	}
	return s
}

// renderCtx is what a slide needs to know about the deck around it.
type renderCtx struct {
	cfg    *Config
	label  string // footer chip
	accent string
}

// render renders one slide. index 0 has no footer (the cover, catalog exercises).
func (s *Slide) render(c renderCtx, index, total int, term string) (string, error) {
	body, err := Blocks(s.Body)
	if err != nil {
		return "", err
	}
	notes := ""
	if strings.TrimSpace(s.Notes) != "" {
		var b strings.Builder
		for _, p := range strings.Split(pyStrip(s.Notes), "\n\n") {
			b.WriteString("<p>" + MD(p) + "</p>")
		}
		notes = "\n<speaker-notes>" + b.String() + "</speaker-notes>"
	}
	tot := total
	if tot < 1 {
		tot = 1
	}
	xp := 100 * float64(index) / float64(tot)
	foot := ""
	if index != 0 {
		trk := ""
		if c.label != "" {
			trk = ` <i class="trk">` + escape(c.label, true) + `</i>`
		}
		foot = `<div class="foot"><span class="brand">` + escape(c.cfg.Brand, false) + trk + `</span>` +
			`<span class="xp" style="--xp:` + fixed(xp, 1) + `%"><b></b></span><span class="lv"><span class="lv-k">LV </span>` + itoa(index) + `/` + itoa(total) + `</span></div>`
	}
	kicker := ""
	if !s.Kicker.Empty() {
		kicker = `<div class="kicker">` + s.Kicker.Render() + `</div>`
	}
	lead := ""
	if !s.Lead.Empty() {
		lead = `<p class="lead">` + s.Lead.Render() + `</p>`
	}
	acc := ` data-accent="` + c.accent + `"`
	dock, tcls := "", ""
	if term != "" {
		dock = termDock(term)
		tcls = " has-term"
	}

	switch s.Layout {
	case "cover":
		var letters strings.Builder
		for _, r := range s.Title.Text {
			if r == ' ' {
				letters.WriteString("<span>&nbsp;</span>")
			} else {
				letters.WriteString("<span>" + escape(string(r), true) + "</span>")
			}
		}
		sc := c.cfg.Cover.Scribbles
		s1, s2 := "", ""
		if len(sc) > 0 {
			s1 = `<span class="scribble s1">` + escape(sc[0], false) + `</span>`
		}
		if len(sc) > 1 {
			s2 = `<span class="scribble s2">` + escape(sc[1], false) + `</span>`
		}
		inner := `<div class="beam"></div><div class="wrap">` + s1 +
			`<div class="hero-title" aria-label="` + escape(s.Title.Text, true) + `">` + letters.String() + `</div><br>` +
			`<div class="hero-sub">` + s.Kicker.Render() + `</div>` +
			`<p class="hero-tag">` + s.Lead.Render() + `</p>` + s2 +
			`<div class="press">` + escape(c.cfg.Cover.Press, false) + `</div></div>`
		return `<section class="slide cover"` + acc + `>` + inner + body + `</section>` + notes, nil
	case "module":
		digits := reNonDigit.ReplaceAllString(s.Num.Text, "")
		if digits == "" {
			digits = "?"
		}
		inner := `<div class="lvlup">LEVEL UP!</div>` +
			`<div class="mission-head"><div class="mission-num">` + zfill2(digits) + `</div><h1>` + s.Title.Render() + `</h1></div>` +
			lead + `<div><span class="rarity">` + s.Num.Render() + `</span></div>`
		return `<section class="slide module"` + acc + `>` + inner + foot + `</section>` + notes, nil
	case "part":
		pn := ""
		if !s.Num.Empty() {
			pn = `<div class="partnum">` + s.Num.Render() + `</div>`
		}
		return `<section class="slide part"` + acc + `>` + pn + `<h1>` + s.Title.Render() + `</h1>` + lead + body + foot + `</section>` + notes, nil
	case "statement":
		if term != "" {
			intro := `<div class="intro">` + kicker + `<h1>` + s.Title.Render() + `</h1>` + lead + body + `</div>`
			return `<section class="slide statement` + tcls + `"` + acc + `><div class="stage">` + intro + dock + `</div>` + foot + `</section>` + notes, nil
		}
		return `<section class="slide statement"` + acc + `>` + kicker + `<h1>` + s.Title.Render() + `</h1>` + lead + body + foot + `</section>` + notes, nil
	}

	// content
	sub := ""
	if !s.Sub.Empty() {
		sub = `<span class="sub">` + s.Sub.Render() + `</span>`
	}
	title := ""
	if !s.Title.Empty() {
		title = `<h1>` + s.Title.Render() + sub + `</h1>`
	}
	timer := ""
	if !s.Timer.Empty() {
		timer = `<span class="timer">` + Icon("timer") + ` ` + s.Timer.Render() + `</span>`
	}
	head := ""
	if kicker != "" || title != "" || timer != "" {
		head = `<div class="head"><div>` + kicker + title + `</div>` + timer + `</div>`
	}
	bcls := "body"
	if s.Center {
		bcls = "body center"
	}
	bodyDiv := `<div class="` + bcls + `">` + lead + body + `</div>`
	if term != "" {
		if s.Lead.Empty() && strings.TrimSpace(body) == "" {
			tcls += " term-only"
			bodyDiv = `<div class="stage">` + dock + `</div>`
		} else {
			bodyDiv = `<div class="stage">` + bodyDiv + dock + `</div>`
		}
	}
	return `<section class="slide` + tcls + `"` + acc + `>` + head + bodyDiv + foot + `</section>` + notes, nil
}
