package deck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var reCharSrc = regexp.MustCompile(`^[\w./-]+\.svg$`)

// characterTag is a rigged SVG character that moves on the slide (respawn.js brings it to
// life: idle motion, blinking, eyes that follow the pointer, a reaction when clicked).
// src is under images/, like ::: img.
func characterTag(src, place, height, extra string) (string, error) {
	if !reCharSrc.MatchString(src) || strings.Contains(src, "..") {
		return "", fmt.Errorf("src=%q: an .svg under images/, e.g. src=themes/welld/d-hello.svg", src)
	}
	cls := "char"
	switch place {
	case "", "inline":
	case "left", "right", "corner", "center":
		cls += " place-" + place
	default:
		return "", fmt.Errorf("place=%q: inline, left, right, center or corner", place)
	}
	if extra != "" {
		cls += " " + extra
	}
	style := ""
	if height != "" {
		h, err := strconv.ParseFloat(height, 64)
		if err != nil || h <= 0 || h > 60 {
			return "", fmt.Errorf("height=%q: a number of slide units, up to 60 (the slide is 56 high)", height)
		}
		style = ` style="--h:` + strconv.FormatFloat(h, 'f', -1, 64) + `"`
	}
	return `<rsp-character class="` + cls + `" data-src="/images/` + src + `"` + style + `></rsp-character>`, nil
}

func characterComponent(p map[string]string) (string, error) {
	return characterTag(p["src"], p["place"], p["height"], "")
}
