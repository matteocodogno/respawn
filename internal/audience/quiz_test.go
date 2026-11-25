package audience

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matteocodogno/respawn/internal/deck"
)

type fakeSrc struct{ Source }

func (fakeSrc) Quiz(id string) *deck.Quiz {
	if id != "q1" {
		return nil
	}
	return &deck.Quiz{ID: "q1", Question: "2+2?", Options: []string{"3", "4", "5"}, Answer: 1, Time: 15, Points: 1000}
}
func (fakeSrc) QuizRules() deck.QuizConfig {
	c := deck.QuizConfig{Time: 15, Points: 1000, Penalty: 250}
	return c
}

func call(t *testing.T, s *Server, method, path, remote string, body any, cookies []*http.Cookie) map[string]any {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.RemoteAddr = remote
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

func TestQuizScoring(t *testing.T) {
	s := New(fakeSrc{}, Options{Deck: "d", StateDir: t.TempDir(), Session: "CODE1", Live: 0})
	const me = "127.0.0.1:1"
	type player struct {
		remote  string
		cookies []*http.Cookie
	}
	join := func(ip, name string) player {
		r := httptest.NewRequest(http.MethodGet, "/join?s=CODE1", nil)
		r.RemoteAddr = ip
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		p := player{ip, w.Result().Cookies()}
		if out := call(t, s, http.MethodPost, "/__quiz/name", ip, map[string]any{"name": name}, p.cookies); out["ok"] != true {
			t.Fatalf("name %s: %v", name, out)
		}
		return p
	}
	fast, slow, wrong, _ := join("10.0.0.1:1", "Fast"), join("10.0.0.2:1", "Slow"), join("10.0.0.3:1", "Wrong"), join("10.0.0.4:1", "Idle")
	if out := call(t, s, http.MethodPost, "/__quiz/name", "10.0.0.5:1", map[string]any{"name": "fast"}, join("10.0.0.5:1", "Other").cookies); out["ok"] == true {
		t.Fatalf("a taken name was accepted")
	}

	play := func(answers map[*player]int, slowMS int) {
		call(t, s, http.MethodPost, "/__quiz/start", me, map[string]any{"id": "q1"}, nil)
		g := s.game
		for p, c := range answers {
			v := call(t, s, http.MethodGet, "/__quiz", p.remote, nil, p.cookies)
			round := v["round"].(map[string]any)
			if round["options"] == nil {
				t.Fatalf("no options for a player")
			}
			if p == &slow { // pretend Slow saw the options a while ago
				g.mu.Lock()
				for k := range g.current().Seen {
					if g.Players[k].Name == "Slow" {
						g.current().Seen[k] -= float64(slowMS) / 1000
					}
				}
				g.mu.Unlock()
			}
			if out := call(t, s, http.MethodPost, "/__quiz/answer", p.remote, map[string]any{"n": round["n"], "choice": c}, p.cookies); out["ok"] != true {
				t.Fatalf("answer: %v", out)
			}
			if out := call(t, s, http.MethodPost, "/__quiz/answer", p.remote, map[string]any{"n": round["n"], "choice": c}, p.cookies); out["ok"] == true {
				t.Fatalf("second answer accepted")
			}
		}
		call(t, s, http.MethodPost, "/__quiz/stop", me, nil, nil)
	}
	score := func(name string) (int, int) {
		for _, p := range s.game.Players {
			if p.Name == name {
				return p.Score, p.Jokers
			}
		}
		t.Fatalf("no %s", name)
		return 0, 0
	}

	for round := 1; round <= 3; round++ {
		play(map[*player]int{&fast: 1, &slow: 1, &wrong: 0}, 12000)
	}
	f, _ := score("Fast")
	sl, _ := score("Slow")
	w, _ := score("Wrong")
	i, jokers := score("Idle")
	if f < 2900 || f > 3000 {
		t.Errorf("Fast: %d, want about 3 x 1000", f)
	}
	if sl >= f || sl < 3*500 {
		t.Errorf("Slow: %d, want less than Fast (%d) and at least 1500", sl, f)
	}
	if w != -750 {
		t.Errorf("Wrong: %d, want -750", w)
	}
	if i != -250 || jokers != 0 {
		t.Errorf("Idle: %d with %d jokers, want two free skips then -250", i, jokers)
	}
	v := call(t, s, http.MethodGet, "/__quiz", me, nil, nil)
	leaders := v["leaders"].([]any)
	if leaders[0].(map[string]any)["name"] != "Fast" {
		t.Errorf("leader: %v", leaders[0])
	}
	call(t, s, http.MethodPost, "/__quiz/reset", me, nil, nil)
	if f, j := score("Fast"); f != 0 || j != 2 {
		t.Errorf("after reset: %d, %d jokers", f, j)
	}
	// the room's copy never carries the options before the start
	if h := audienceHTML(`<head></head><div class="quiz"><quiz-options><ol>x</ol></quiz-options></div>`, 0, 0); bytes.Contains([]byte(h), []byte("quiz-options")) {
		t.Errorf("options in the room's copy")
	}
}
