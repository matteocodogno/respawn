package deck

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// escape mirrors Python's html.escape: & < > always, " and ' when quote is true.
func escape(s string, quote bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			if quote {
				b.WriteString("&quot;")
			} else {
				b.WriteRune(r)
			}
		case '\'':
			if quote {
				b.WriteString("&#x27;")
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	reCode   = regexp.MustCompile("`([^`]+)`")
	reStrong = regexp.MustCompile(`\*\*(.+?)\*\*`)
)

// Inline is a piece of slide text: Markdown-ish text, or HTML passed through as is.
type Inline struct {
	Text string
	HTML bool
}

// MD renders inline text: HTML-escaped, with **bold**, `code`, *accent* and newline -> <br>.
func MD(text string) string {
	s := escape(text, false)
	s = reCode.ReplaceAllString(s, "<code>$1</code>")
	s = reStrong.ReplaceAllString(s, "<strong>$1</strong>")
	s = emphasis(s)
	return strings.ReplaceAll(s, "\n", "<br>")
}

// Render renders an inline value.
func (i Inline) Render() string {
	if i.HTML {
		return i.Text
	}
	return MD(i.Text)
}

func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// emphasis implements (?<![\w*])\*(?!\s)(.+?)(?<!\s)\*(?![\w*]) without look-arounds.
func emphasis(s string) string {
	runes := []rune(s)
	var b strings.Builder
	i := 0
	for i < len(runes) {
		if runes[i] == '*' && (i == 0 || !(isWord(runes[i-1]) || runes[i-1] == '*')) &&
			i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) && runes[i+1] != '\n' {
			// lazy: the first closing star that satisfies the conditions, at least one char inside
			end := -1
			for j := i + 2; j < len(runes); j++ {
				if runes[j-1] == '\n' {
					break // "." does not match newlines
				}
				if runes[j] == '\n' {
					break
				}
				if runes[j] == '*' && !unicode.IsSpace(runes[j-1]) &&
					(j+1 >= len(runes) || !(isWord(runes[j+1]) || runes[j+1] == '*')) {
					end = j
					break
				}
			}
			if end > 0 {
				b.WriteString("<em>")
				b.WriteString(string(runes[i+1 : end]))
				b.WriteString("</em>")
				i = end + 1
				continue
			}
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}

// Icon is a Material Symbols glyph.
func Icon(name string) string {
	return `<span class="ms">` + escape(name, true) + `</span>`
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }
