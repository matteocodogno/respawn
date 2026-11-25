package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/matteocodogno/respawn/internal/audience"
)

func showcase(t *testing.T) *Server {
	t.Helper()
	s, err := New(Options{Dir: "../../examples/showcase", Deck: "workshop", Out: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func get(t *testing.T, h http.Handler, path, remote string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.RemoteAddr = remote
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPresenter(t *testing.T) {
	s := showcase(t)
	h := s.handler(nil)
	w := get(t, h, "/3", "127.0.0.1:5000")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `window.RESPAWN = {`) || !strings.Contains(w.Body.String(), `"ns":"showcase"`) {
		t.Fatalf("slide 3: %d %.300s", w.Code, w.Body.String())
	}
	for _, p := range []string{"/", "/style.css", "/js/respawn.js", "/media/search-workshop.json", "/media/search-talk.json", "/media/exercises.json", "/media/print.html"} {
		if w := get(t, h, p, "127.0.0.1:5000"); w.Code != 200 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := get(t, h, "/999", "127.0.0.1:5000"); w.Code != 404 {
		t.Errorf("/999: %d", w.Code)
	}
	if w := get(t, h, "/shell/workspace", "127.0.0.1:5000"); w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "/terminal?cmd=") {
		t.Errorf("/shell: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := get(t, h, "/shell/..%2f..", "127.0.0.1:5000"); w.Code == http.StatusSeeOther {
		t.Errorf("a shell outside the presentation opened")
	}
}

func TestAudience(t *testing.T) {
	s := showcase(t)
	a := audience.New(s, audience.Options{Deck: "workshop", Port: 8891, StateDir: t.TempDir(), Session: "ABCDE", Live: 2, Title: "Show"})
	const room, me = "10.0.0.7:4000", "127.0.0.1:4000"

	if w := get(t, a, "/2", room); !strings.Contains(w.Body.String(), "type the session code") {
		t.Fatalf("no join page before joining: %d", w.Code)
	}
	w := get(t, a, "/join?s=abcde", room)
	if w.Code != http.StatusFound {
		t.Fatalf("join: %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("join cookies: %v", cookies)
	}
	w = get(t, a, "/2", room, cookies...)
	body := w.Body.String()
	if w.Code != 200 || strings.Contains(body, "<speaker-notes>") || !strings.Contains(body, "RESPAWN_AUDIENCE = { live: 2 }") {
		t.Fatalf("slide 2 for the room: %d %.200s", w.Code, body)
	}
	if !strings.Contains(body, `const NextURL = '\/2';`) && !strings.Contains(body, `const NextURL = '/2';`) {
		t.Errorf("the live slide still has a next slide")
	}
	if w := get(t, a, "/5", room, cookies...); w.Code != http.StatusFound || w.Header().Get("Location") != "/2" {
		t.Errorf("ahead of the presenter: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := get(t, a, "/__live/set?n=5", room, cookies...); w.Code != http.StatusForbidden {
		t.Errorf("the room moved the slides: %d", w.Code)
	}
	if w := get(t, a, "/__live/set?n=5", me); w.Code != http.StatusNoContent {
		t.Errorf("presenter set: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "/__live/set?n=6", nil)
	r.RemoteAddr = me
	r.Header.Set("Cf-Connecting-Ip", "1.2.3.4") // through the tunnel: never the presenter
	w = httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("tunnel request treated as the presenter: %d", w.Code)
	}
	if w := get(t, a, "/media/search-workshop.json", room, cookies...); strings.Contains(w.Body.String(), `"n":6`) || strings.Contains(w.Body.String(), `"o":`) {
		t.Errorf("search index leaks: %.200s", w.Body.String())
	}
	if w := get(t, a, "/media/notes.html", room, cookies...); w.Code != 404 {
		t.Errorf("notes page for the room: %d", w.Code)
	}
	if w := get(t, a, "/__session", me); !strings.Contains(w.Body.String(), url.QueryEscape("ABCDE")) {
		t.Errorf("session: %s", w.Body.String())
	}
}

func TestTheme(t *testing.T) {
	s := showcase(t)
	if h, _ := s.Page(1); strings.Contains(h, "/themes/") {
		t.Fatalf("default look loads a theme")
	}
	s.proj.Config.Theme = "clean"
	s.pages = map[int]string{}
	if err := CheckTheme(s.proj.Config); err != nil {
		t.Fatal(err)
	}
	if h, _ := s.Page(1); !strings.Contains(h, `href="/themes/clean.css?hash=`) {
		t.Fatalf("clean theme not loaded")
	}
	if w := get(t, s.handler(nil), "/themes/clean.css", "127.0.0.1:5000"); w.Code != 200 {
		t.Fatalf("theme file: %d", w.Code)
	}
	s.proj.Config.Theme = "nope"
	if err := CheckTheme(s.proj.Config); err == nil || !strings.Contains(err.Error(), "clean") {
		t.Fatalf("unknown theme: %v", err)
	}
}
