package deck

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// pyStr formats a value as Python's str() would (numbers as written in the slides).
func pyStr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return pyFloat(t)
	case bool:
		if t {
			return "True"
		}
		return "False"
	}
	return fmt.Sprint(v)
}

// pyFloat is Python's repr() of a float.
func pyFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if math.IsNaN(f) {
		return "nan"
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e16 {
		return strconv.FormatFloat(f, 'f', 1, 64)
	}
	abs := math.Abs(f)
	if abs >= 1e16 || abs < 1e-4 {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		// Go: 1e-05 / 1.5e+16 ; Python: 1e-05 / 1.5e+16 (same exponent style, two digits min)
		return s
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// fixed formats like Python's f"{x:.Nf}".
func fixed(f float64, n int) string {
	return strconv.FormatFloat(f, 'f', n, 64)
}

// pyJSON is Python's json.dumps(v) with default settings: ", " and ": " separators and
// ensure_ascii (non-ASCII as \uXXXX, surrogate pairs above the BMP).
func pyJSON(v any) string {
	var b strings.Builder
	writeJSON(&b, v)
	return b.String()
}

func writeJSON(b *strings.Builder, v any) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		b.WriteString(strconv.Itoa(t))
	case float64:
		b.WriteString(pyFloat(t))
	case string:
		writeJSONString(b, t)
	case []any:
		b.WriteString("[")
		for i, x := range t {
			if i > 0 {
				b.WriteString(", ")
			}
			writeJSON(b, x)
		}
		b.WriteString("]")
	case orderedMap:
		b.WriteString("{")
		for i, k := range t.keys {
			if i > 0 {
				b.WriteString(", ")
			}
			writeJSONString(b, k)
			b.WriteString(": ")
			writeJSON(b, t.vals[k])
		}
		b.WriteString("}")
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		writeJSON(b, orderedMap{keys: keys, vals: t})
	default:
		writeJSONString(b, fmt.Sprint(t))
	}
}

type orderedMap struct {
	keys []string
	vals map[string]any
}

func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r > 0x7e {
				if r > 0xffff {
					r1, r2 := utf16.EncodeRune(r)
					fmt.Fprintf(b, `\u%04x\u%04x`, r1, r2)
				} else {
					fmt.Fprintf(b, `\u%04x`, r)
				}
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
