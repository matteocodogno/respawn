package audience

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/matteocodogno/respawn/internal/deck"
)

// The live quiz: the presenter starts a question of the slides ("::: quiz"), every player
// has the question's seconds to answer on their phone, counted from the moment their
// phone got the options. Scores:
//
//   - right: Points if answered at once, down to half of it at the last second;
//   - wrong: -Penalty;
//   - no answer: a joker if the player has one left (Jokers per game), else -Penalty.
//
// Players are the people of the room who gave a name; the game lives in .respawn/ until
// the presenter resets it.

// QuizSource is what the quiz needs from the deck.
type QuizSource interface {
	Quiz(id string) *deck.Quiz
	QuizRules() deck.QuizConfig
}

// grace is how long after its time a round still takes answers: the options reach the
// phones a little after the start, and each phone has its own full time.
const grace = 1.5

// Player is someone of the room in the game.
type Player struct {
	Name    string `json:"name"`
	Score   int    `json:"score"`
	Jokers  int    `json:"jokers"`
	Right   int    `json:"right"`
	Since   int    `json:"since"` // first round they play
	LastSaw int64  `json:"-"`
}

// Answer is a player's answer to a round.
type Answer struct {
	Choice int `json:"choice"`
	MS     int `json:"ms"`
}

// Result is what a round did to a player's score.
type Result struct {
	Delta int    `json:"delta"`
	Kind  string `json:"kind"` // right, wrong, joker, missed
	MS    int    `json:"ms,omitempty"`
}

// Round is one question played.
type Round struct {
	N       int                `json:"n"`
	ID      string             `json:"id"`
	Answer  int                `json:"answer"`
	Options int                `json:"options"`
	Time    int                `json:"time"`
	Points  int                `json:"points"`
	Start   float64            `json:"start"`
	Ends    float64            `json:"ends"` // last moment an answer is taken
	Players []string           `json:"players"`
	Seen    map[string]float64 `json:"seen"`
	Answers map[string]Answer  `json:"answers"`
	Results map[string]Result  `json:"results,omitempty"`
	Scored  bool               `json:"scored"`
}

type game struct {
	mu      sync.Mutex
	Players map[string]*Player `json:"players"`
	Rounds  []*Round           `json:"rounds"`
	Board   bool               `json:"board"` // leaderboard on the room's screens
}

func (s *Server) quizSource() (QuizSource, bool) {
	q, ok := s.src.(QuizSource)
	return q, ok
}

func (s *Server) loadGame() {
	g := &game{Players: map[string]*Player{}}
	s.st.load("quiz", g)
	if g.Players == nil {
		g.Players = map[string]*Player{}
	}
	s.game = g
}

// call with g.mu held
func (s *Server) saveGame() { s.st.save("quiz", s.game) }

// current is the last round, or nil.
func (g *game) current() *Round {
	if len(g.Rounds) == 0 {
		return nil
	}
	return g.Rounds[len(g.Rounds)-1]
}

// settle scores a round whose time is over. Call with g.mu held.
func (s *Server) settle(r *Round, rules deck.QuizConfig) bool {
	if r == nil || r.Scored || now() < r.Ends {
		return false
	}
	g := s.game
	r.Results = map[string]Result{}
	who := map[string]bool{}
	for _, v := range r.Players {
		who[v] = true
	}
	for v := range r.Answers {
		who[v] = true
	}
	for v := range who {
		p := g.Players[v]
		if p == nil {
			continue
		}
		var res Result
		if a, ok := r.Answers[v]; ok {
			res.MS = a.MS
			if a.Choice == r.Answer {
				frac := 1 - float64(a.MS)/float64(r.Time*1000)
				frac = math.Max(0, math.Min(1, frac))
				res.Kind, res.Delta = "right", int(math.Round(float64(r.Points)*(0.5+0.5*frac)))
				p.Right++
			} else {
				res.Kind, res.Delta = "wrong", -rules.Penalty
			}
		} else if p.Jokers > 0 {
			p.Jokers--
			res.Kind = "joker"
		} else {
			res.Kind, res.Delta = "missed", -rules.Penalty
		}
		p.Score += res.Delta
		r.Results[v] = res
	}
	r.Scored = true
	return true
}

type boardRow struct {
	Rank  int    `json:"rank"`
	Name  string `json:"name"`
	Score int    `json:"score"`
	Delta int    `json:"delta"`
	Right int    `json:"right"`
	Me    bool   `json:"me,omitempty"`
}

// board is the leaderboard, best first; ties share the rank. Call with g.mu held.
func (g *game) board(me string) []boardRow {
	last := g.current()
	rows := []boardRow{}
	for v, p := range g.Players {
		row := boardRow{Name: p.Name, Score: p.Score, Right: p.Right, Me: v == me}
		if last != nil && last.Scored {
			row.Delta = last.Results[v].Delta
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	for i := range rows {
		rows[i].Rank = i + 1
		if i > 0 && rows[i].Score == rows[i-1].Score {
			rows[i].Rank = rows[i-1].Rank
		}
	}
	return rows
}

func (s *Server) quizPost(w http.ResponseWriter, path string, local bool, viewer string, data map[string]any) {
	src, ok := s.quizSource()
	if !ok {
		text(w, http.StatusNotFound, "no quiz here")
		return
	}
	rules := src.QuizRules()
	g := s.game
	g.mu.Lock()
	defer g.mu.Unlock()
	s.settle(g.current(), rules)
	only := func() bool {
		if !local {
			text(w, http.StatusForbidden, "only the presenter's computer runs the quiz")
		}
		return local
	}
	switch path {
	case "/__quiz/start": // a question of the slides, now
		if !only() {
			return
		}
		q := src.Quiz(str(data["id"]))
		if q == nil {
			send(w, http.StatusNotFound, []byte(`{"ok": false, "error": "no such quiz in this deck"}`), "application/json", true)
			return
		}
		if r := g.current(); r != nil && !r.Scored { // a new question closes the one still open
			r.Ends = now()
			s.settle(r, rules)
		}
		r := &Round{N: len(g.Rounds) + 1, ID: q.ID, Answer: q.Answer, Options: len(q.Options), Time: q.Time,
			Points: q.Points, Start: now(), Seen: map[string]float64{}, Answers: map[string]Answer{}}
		r.Ends = r.Start + float64(q.Time) + grace
		for v := range g.Players {
			r.Players = append(r.Players, v)
		}
		sort.Strings(r.Players)
		g.Rounds = append(g.Rounds, r)
		g.Board = false
		s.saveGame()
		sendJSON(w, http.StatusOK, s.presenterView(), true)
	case "/__quiz/stop": // close the answers now
		if !only() {
			return
		}
		if r := g.current(); r != nil && !r.Scored {
			r.Ends = now()
			s.settle(r, rules)
			s.saveGame()
		}
		sendJSON(w, http.StatusOK, s.presenterView(), true)
	case "/__quiz/board": // the leaderboard on the room's phones too
		if !only() {
			return
		}
		g.Board = truthy(data["on"])
		s.saveGame()
		sendJSON(w, http.StatusOK, s.presenterView(), true)
	case "/__quiz/reset": // a new game: scores, jokers and rounds go; names stay unless all
		if !only() {
			return
		}
		if truthy(data["all"]) {
			g.Players = map[string]*Player{}
		}
		for _, p := range g.Players {
			p.Score, p.Right, p.Jokers, p.Since = 0, 0, rules.FreeSkips(), 1
		}
		g.Rounds, g.Board = nil, false
		s.saveGame()
		sendJSON(w, http.StatusOK, s.presenterView(), true)
	case "/__quiz/name": // a player of the room picks a name
		if local {
			text(w, http.StatusForbidden, "the presenter does not play")
			return
		}
		name := clip(strings.TrimSpace(reSpaces.ReplaceAllString(str(data["name"]), " ")), 24)
		if name == "" {
			send(w, http.StatusBadRequest, []byte(`{"ok": false, "error": "a name, please"}`), "application/json", false)
			return
		}
		for v, p := range g.Players {
			if v != viewer && strings.EqualFold(p.Name, name) {
				send(w, http.StatusConflict, []byte(`{"ok": false, "error": "taken: pick another name"}`), "application/json", false)
				return
			}
		}
		if p := g.Players[viewer]; p != nil {
			p.Name = name
		} else {
			g.Players[viewer] = &Player{Name: name, Jokers: rules.FreeSkips(), Since: len(g.Rounds) + 1}
		}
		s.saveGame()
		send(w, http.StatusOK, []byte(`{"ok": true}`), "application/json", false)
	case "/__quiz/answer":
		r := g.current()
		n, _ := num(data["n"])
		choice, okc := num(data["choice"])
		p := g.Players[viewer]
		t := now()
		switch {
		case local || p == nil:
			text(w, http.StatusForbidden, "pick a name first")
		case r == nil || r.N != n || r.Scored || !okc || choice < 0 || choice >= r.Options:
			sendJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "this question is closed"}, false)
		default:
			if _, done := r.Answers[viewer]; done {
				sendJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "one answer per question"}, false)
				return
			}
			seen, ok := r.Seen[viewer]
			if !ok {
				seen = r.Start
			}
			if t > math.Min(seen+float64(r.Time)+0.5, r.Ends) {
				sendJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "time is up"}, false)
				return
			}
			r.Answers[viewer] = Answer{Choice: choice, MS: int((t - seen) * 1000)}
			s.saveGame()
			sendJSON(w, http.StatusOK, map[string]any{"ok": true}, false)
		}
	default:
		text(w, http.StatusNotFound, "not here")
	}
}

// presenterView is the round as the presenter's screen shows it. Call with g.mu held.
func (s *Server) presenterView() map[string]any {
	g := s.game
	out := map[string]any{"players": len(g.Players), "board": g.Board, "leaders": g.board("")}
	r := g.current()
	if r == nil {
		return out
	}
	t := now()
	view := map[string]any{"n": r.N, "id": r.ID, "time": r.Time, "open": !r.Scored,
		"left": math.Max(0, r.Start+float64(r.Time)-t), "answered": len(r.Answers), "players": len(r.Players)}
	if r.Scored {
		counts := make([]int, r.Options)
		for _, a := range r.Answers {
			counts[a.Choice]++
		}
		type fast struct {
			Name string `json:"name"`
			MS   int    `json:"ms"`
		}
		var fastest []fast
		for v, res := range r.Results {
			if res.Kind == "right" && g.Players[v] != nil {
				fastest = append(fastest, fast{g.Players[v].Name, res.MS})
			}
		}
		sort.Slice(fastest, func(i, j int) bool { return fastest[i].MS < fastest[j].MS })
		if len(fastest) > 3 {
			fastest = fastest[:3]
		}
		view["answer"], view["counts"], view["fastest"] = r.Answer, counts, fastest
	}
	out["round"] = view
	return out
}

// playerView is the round as one player's phone shows it. Call with g.mu held.
func (s *Server) playerView(viewer string, src QuizSource) map[string]any {
	g := s.game
	p := g.Players[viewer]
	out := map[string]any{"board": g.Board}
	if p == nil {
		out["name"] = nil
	} else {
		out["name"], out["score"], out["jokers"] = p.Name, p.Score, p.Jokers
		for _, row := range g.board(viewer) {
			if row.Me {
				out["rank"] = row.Rank
			}
		}
		out["players"] = len(g.Players)
	}
	if g.Board {
		rows := g.board(viewer)
		top := rows
		if len(top) > 10 {
			top = top[:10]
		}
		out["leaders"] = top
	}
	r := g.current()
	if r == nil {
		return out
	}
	q := src.Quiz(r.ID)
	t := now()
	view := map[string]any{"n": r.N, "time": r.Time, "open": !r.Scored}
	if q != nil {
		view["q"], view["slide"] = q.Question, q.Slide
	}
	if p != nil && !r.Scored {
		seen, ok := r.Seen[viewer]
		if !ok {
			seen = t
			r.Seen[viewer] = seen
		}
		deadline := math.Min(seen+float64(r.Time), r.Ends)
		view["left"] = math.Max(0, deadline-t)
	}
	if q != nil && (p != nil || r.Scored) {
		view["options"] = q.Options
	}
	if a, ok := r.Answers[viewer]; ok {
		view["mine"] = a.Choice
	}
	if r.Scored {
		view["answer"] = r.Answer
		if q != nil && q.Explain != "" {
			view["explain"] = q.Explain
		}
		if res, ok := r.Results[viewer]; ok {
			view["result"] = res
		}
	}
	out["round"] = view
	return out
}

func (s *Server) quizGet(w http.ResponseWriter, r *http.Request, local bool) {
	src, ok := s.quizSource()
	if !ok {
		sendJSON(w, http.StatusOK, map[string]any{}, true)
		return
	}
	g := s.game
	g.mu.Lock()
	defer g.mu.Unlock()
	if s.settle(g.current(), src.QuizRules()) {
		s.saveGame()
	}
	if local {
		sendJSON(w, http.StatusOK, s.presenterView(), true)
		return
	}
	viewer := s.viewer(r)
	if p := g.Players[viewer]; p != nil {
		p.LastSaw = time.Now().Unix()
	}
	sendJSON(w, http.StatusOK, s.playerView(viewer, src), false)
}
