package deck

import "testing"

func TestMD(t *testing.T) {
	cases := map[string]string{
		"a **b** c":            "a <strong>b</strong> c",
		"`x<y`":                "<code>x&lt;y</code>",
		"an *accent* here":     "an <em>accent</em> here",
		"5 * 3 * 2":            "5 * 3 * 2",
		"line\nbreak":          "line<br>break",
		"a*b*c":                "a*b*c",
		"*one* and *two*":      "<em>one</em> and <em>two</em>",
		"it's \"quoted\" & co": "it's \"quoted\" &amp; co",
	}
	for in, want := range cases {
		if got := MD(in); got != want {
			t.Errorf("MD(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPyJSON(t *testing.T) {
	got := pyJSON(orderedMap{keys: []string{"a", "b"}, vals: map[string]any{"a": []any{"🥐", 2.2, 3}, "b": 0.7}})
	want := "{\"a\": [\"\\ud83e\\udd50\", 2.2, 3], \"b\": 0.7}"
	if got != want {
		t.Errorf("pyJSON = %s, want %s", got, want)
	}
}

func TestCheckVersion(t *testing.T) {
	for _, tc := range []struct {
		want, have string
		ok         bool
	}{
		{">=0.1", "0.1.0", true}, {">=0.2", "0.1.9", false}, {">=0.1, <1.0", "1.0.0", false},
		{"^0.3", "0.3.4", true}, {"^0.3", "0.4.0", false}, {"^1.2", "1.9.0", true}, {"~1.2", "1.3.0", false},
		{">=9", "dev", true}, {"", "0.1.0", true}, {"0.2.1", "v0.2.1", true},
	} {
		c := &Config{Respawn: tc.want}
		if err := c.CheckVersion(tc.have); (err == nil) != tc.ok {
			t.Errorf("%q with %s: %v", tc.want, tc.have, err)
		}
	}
}
