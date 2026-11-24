package deck

import (
	"fmt"
	"strconv"
	"strings"
)

// CheckVersion tells whether this respawn (version) satisfies the presentation's
// "respawn:" constraint, e.g. ">=0.2, <1.0". Development builds satisfy everything.
func (c *Config) CheckVersion(version string) error {
	if c.Respawn == "" || version == "" || version == "dev" {
		return nil
	}
	have, ok := parseVersion(version)
	if !ok {
		return nil
	}
	for _, part := range strings.Split(c.Respawn, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		op := ""
		for _, o := range []string{">=", "<=", "==", "!=", ">", "<", "=", "^", "~"} {
			if strings.HasPrefix(part, o) {
				op = o
				break
			}
		}
		want, ok := parseVersion(strings.TrimSpace(part[len(op):]))
		if !ok {
			return fmt.Errorf("%s: cannot read the version constraint %q", ConfigFile, part)
		}
		cmp := compareVersions(have, want)
		good := true
		switch op {
		case ">=":
			good = cmp >= 0
		case "<=":
			good = cmp <= 0
		case ">":
			good = cmp > 0
		case "<":
			good = cmp < 0
		case "!=":
			good = cmp != 0
		case "^": // same major (same minor below 1.0)
			good = cmp >= 0 && have[0] == want[0] && (want[0] > 0 || have[1] == want[1])
		case "~": // same minor
			good = cmp >= 0 && have[0] == want[0] && have[1] == want[1]
		default:
			good = cmp == 0
		}
		if !good {
			return fmt.Errorf("this presentation needs respawn %s, this is %s: brew upgrade respawn", c.Respawn, version)
		}
	}
	return nil
}

func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 || parts[0] == "" {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func compareVersions(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
