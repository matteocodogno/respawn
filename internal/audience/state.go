// Package audience is the room's copy of the deck: read-only slides up to the presenter's,
// sessions with a join code, polls, questions, breaks, exercises, hold and theme.
//
// It listens on its own port (the presenter's + 2) on every interface. Requests from the
// presenter's computer (loopback, not relayed by a Cloudflare tunnel) drive it; everybody
// else must join with the session code first and only ever reads.
package audience

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// codeChars has no look-alike characters (0/O, 1/I/L): easy to read off a slide.
const codeChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		k, _ := rand.Int(rand.Reader, big.NewInt(int64(len(codeChars))))
		b[i] = codeChars[k.Int64()]
	}
	return string(b)
}

// Exercise is the catalog exercise the presenter is running.
type Exercise struct {
	ID   string `json:"id"`
	Step int    `json:"step"`
}

// Break is the presenter's coffee or lunch break.
type Break struct {
	Mode    string  `json:"mode"`
	Running bool    `json:"running"`
	Done    bool    `json:"done"`
	Total   int     `json:"total"`
	Left    int     `json:"left"`
	At      string  `json:"at"`
	End     float64 `json:"-"`
}

// Poll is one question to the room.
type Poll struct {
	ID    int            `json:"id"`
	Q     string         `json:"q"`
	Opts  []string       `json:"opts"`
	Ends  float64        `json:"ends"`
	Votes map[string]int `json:"votes"`
}

// PollRecord is a closed poll kept for later.
type PollRecord struct {
	ID      int      `json:"id"`
	Q       string   `json:"q"`
	Opts    []string `json:"opts"`
	Counts  []int    `json:"counts"`
	Total   int      `json:"total"`
	At      string   `json:"at"`
	Current bool     `json:"current,omitempty"`
	Open    bool     `json:"open,omitempty"`
}

// Question is a question from the room.
type Question struct {
	ID    int    `json:"id"`
	Text  string `json:"text"`
	Name  string `json:"name"`
	Slide int    `json:"slide"`
	At    string `json:"at"`
	Seen  bool   `json:"seen"`
	Done  bool   `json:"done"`
}

// State is everything the room shares; persisted under .respawn/ in the presentation.
type State struct {
	mu sync.Mutex

	Code  string
	Live  int
	Held  bool
	Ex    *Exercise
	Brk   *Break
	Theme string
	seen  map[string]time.Time
	lastQ map[string]time.Time
	Poll  Poll
	Polls []PollRecord
	Qs    []Question
	Show  int // the question over the slide, 0 for none
	dir   string
	deck  string
}

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func newState(dir, deck string) *State {
	return &State{dir: dir, deck: deck, seen: map[string]time.Time{}, lastQ: map[string]time.Time{},
		Poll: Poll{Votes: map[string]int{}}}
}

func (s *State) path(kind string) string {
	return filepath.Join(s.dir, kind+"-"+s.deck+".json")
}

func (s *State) save(kind string, v any) {
	if s.dir == "" {
		return
	}
	_ = os.MkdirAll(s.dir, 0o755)
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	tmp := s.path(kind) + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, s.path(kind))
	}
}

func (s *State) load(kind string, v any) bool {
	data, err := os.ReadFile(s.path(kind))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

type sessionFile struct {
	Code  string  `json:"code"`
	Live  int     `json:"live"`
	Saved float64 `json:"saved"`
}

type pollsFile struct {
	Current Poll         `json:"current"`
	History []PollRecord `json:"history"`
}

type questionsFile struct {
	Items   []Question `json:"items"`
	Showing int        `json:"showing"`
}

// call with mu held
func (s *State) saveSession() {
	s.save("session", sessionFile{Code: s.Code, Live: s.Live, Saved: now()})
}
func (s *State) savePolls() { s.save("polls", pollsFile{Current: s.Poll, History: s.Polls}) }
func (s *State) saveQs()    { s.save("questions", questionsFile{Items: s.Qs, Showing: s.Show}) }

func (s *State) restore(resume bool) {
	if resume {
		var sf sessionFile
		if s.load("session", &sf) {
			s.Code, s.Live = sf.Code, sf.Live
		}
	}
	var pf pollsFile
	if s.load("polls", &pf) {
		if pf.Current.ID != 0 {
			s.Poll = pf.Current
			if s.Poll.Votes == nil {
				s.Poll.Votes = map[string]int{}
			}
		}
		s.Polls = pf.History
	}
	var qf questionsFile
	if s.load("questions", &qf) {
		s.Qs, s.Show = qf.Items, qf.Showing
	}
}

func counts(p Poll) []int {
	c := make([]int, len(p.Opts))
	for _, v := range p.Votes {
		if v >= 0 && v < len(c) {
			c[v]++
		}
	}
	return c
}

func record(p Poll) PollRecord {
	end := p.Ends
	if n := now(); n < end {
		end = n
	}
	return PollRecord{ID: p.ID, Q: p.Q, Opts: p.Opts, Counts: counts(p), Total: len(p.Votes),
		At: time.Unix(int64(end), 0).Format("15:04")}
}
