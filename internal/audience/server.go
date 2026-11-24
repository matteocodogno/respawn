package audience

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/matteocodogno/respawn/internal/deck"
)

// Source is the presenter's side: the rendered slides and what is built with them.
type Source interface {
	Page(n int) (string, bool) // the presenter's page of slide n
	Index() []deck.IndexRow    // search index of the deck
	Catalog() []*deck.Exercise // the shared exercise catalog
	Static() http.Handler      // assets (style, js, fonts, images, media)
}

// Options are the audience copy's settings.
type Options struct {
	Deck     string
	Port     int
	StateDir string // .respawn/ in the presentation
	Public   string // public address the room uses (Cloudflare Tunnel), e.g. https://live.example.dev
	Session  string // session code; default a new one (or the last one with Resume)
	Resume   bool   // same code and live slide as the last run
	Live     int    // starting slide, -1 for the default
	Title    string // shown on the join page
}

// Server is the audience copy of one deck.
type Server struct {
	src Source
	o   Options
	st  *State
	lan string
}

// New prepares the audience copy; ListenAndServe starts it.
func New(src Source, o Options) *Server {
	st := newState(o.StateDir, o.Deck)
	st.restore(o.Resume)
	code := strings.ToUpper(o.Session)
	if code == "" {
		code = st.Code
	}
	code = reNotCode.ReplaceAllString(strings.ToUpper(code), "")
	if len(code) > 12 {
		code = code[:12]
	}
	if code == "" {
		code = newCode(5)
	}
	st.Code = code
	if o.Live >= 0 {
		st.Live = o.Live
	}
	st.saveSession()
	return &Server{src: src, o: o, st: st, lan: lanIP()}
}

var reNotCode = regexp.MustCompile(`[^A-Z0-9]`)

// Code is the session code.
func (s *Server) Code() string { return s.st.Code }

// Base is the address the room uses.
func (s *Server) Base() string {
	if s.o.Public != "" {
		return strings.TrimRight(s.o.Public, "/")
	}
	return fmt.Sprintf("http://%s:%d", s.lan, s.o.Port)
}

// JoinURL is what the QR code holds.
func (s *Server) JoinURL() string { return s.Base() + "/join?s=" + s.st.Code }

// ListenAndServe serves the room on every interface.
func (s *Server) ListenAndServe() error {
	srv := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", s.o.Port), Handler: s,
		ReadHeaderTimeout: 10 * time.Second, ErrorLog: quietLog()}
	return srv.ListenAndServe()
}

func lanIP() string {
	c, err := net.Dial("udp", "10.255.255.255:1")
	if err != nil {
		return "<your-ip>"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// isLocal: the presenter's own browser, on this computer and not relayed by the tunnel
// (cloudflared connects from 127.0.0.1 too, but always adds Cf-Connecting-Ip).
func isLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() && r.Header.Get("Cf-Connecting-Ip") == ""
}

var reViewer = regexp.MustCompile(`[^a-z0-9]`)

// viewer is the viewer id of a participant who joined this session, or "".
func (s *Server) viewer(r *http.Request) string {
	c, err := r.Cookie("respawn_s")
	if err != nil || s.st.Code == "" || c.Value != s.st.Code {
		return ""
	}
	v := ""
	if vc, err := r.Cookie("respawn_v"); err == nil {
		v = reViewer.ReplaceAllString(vc.Value, "")
		if len(v) > 32 {
			v = v[:32]
		}
	}
	if v != "" {
		s.st.mu.Lock()
		s.st.seen[v] = time.Now()
		s.st.mu.Unlock()
	}
	return v
}

func send(w http.ResponseWriter, code int, body []byte, ctype string, cors bool) {
	h := w.Header()
	if ctype == "" {
		ctype = "text/plain; charset=utf-8"
	}
	h.Set("Content-Type", ctype)
	h.Set("Cache-Control", "no-store")
	if cors {
		h.Set("Access-Control-Allow-Origin", "*")
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func sendJSON(w http.ResponseWriter, code int, v any, cors bool) {
	data, _ := json.Marshal(v)
	send(w, code, data, "application/json", cors)
}

func text(w http.ResponseWriter, code int, msg string) { send(w, code, []byte(msg), "", false) }

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.URL.Query().Get("s"))
	if !regexp.MustCompile(`^[A-Z0-9]{1,12}$`).MatchString(code) {
		code = ""
	}
	if code != "" && code == s.st.Code {
		v := ""
		if c, err := r.Cookie("respawn_v"); err == nil {
			v = reViewer.ReplaceAllString(c.Value, "")
		}
		if v == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			v = hex.EncodeToString(b)
		}
		for _, c := range []*http.Cookie{{Name: "respawn_s", Value: code}, {Name: "respawn_v", Value: v}} {
			c.MaxAge, c.Path, c.SameSite, c.HttpOnly = 86400, "/", http.SameSiteLaxMode, true
			http.SetCookie(w, c)
		}
		w.Header().Set("Location", "/")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusFound)
		return
	}
	errHTML, status := "", http.StatusOK
	if code != "" {
		errHTML, status = `<p class="err">That code is not the one on the screen. Try again.</p>`, http.StatusForbidden
	}
	page := strings.ReplaceAll(joinPage, "{err}", errHTML)
	page = strings.ReplaceAll(page, "{title}", htmlEscape(s.o.Title))
	send(w, status, []byte(page), "text/html; charset=utf-8", false)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func (s *Server) pollView(local bool) map[string]any {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	p := s.st.Poll
	left := p.Ends - now()
	if left < 0 {
		left = 0
	}
	opts := p.Opts
	if opts == nil {
		opts = []string{}
	}
	out := map[string]any{"id": p.ID, "q": p.Q, "opts": opts, "open": left > 0,
		"left": float64(int(left*100+0.5)) / 100, "total": len(p.Votes)}
	if local || left <= 0 { // the room sees the results once the poll is closed
		out["counts"] = counts(p)
	}
	return out
}

// ServeHTTP is the whole audience copy.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = recover() }() // a browser that went away mid-answer is normal
	if r.Method == http.MethodOptions {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodPost {
		s.post(w, r)
		return
	}
	s.get(w, r)
}

func (s *Server) body(r *http.Request) map[string]any {
	if r.ContentLength <= 0 || r.ContentLength > 20000 {
		return map[string]any{}
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 20000))
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if json.Unmarshal(data, &out) != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprint(t)
	}
}

func num(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		return n, err == nil
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}

var reSpaces = regexp.MustCompile(`\s+`)

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func (s *Server) post(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	local := isLocal(r)
	viewer := ""
	if !local {
		viewer = s.viewer(r)
		if viewer == "" {
			text(w, http.StatusForbidden, "join the session first")
			return
		}
	}
	data := s.body(r)
	st := s.st
	only := func(what string) bool {
		if !local {
			text(w, http.StatusForbidden, "only the presenter's computer "+what)
		}
		return local
	}
	switch path {
	case "/__break": // the presenter's coffee or lunch break, shown to the room too
		if !only("runs breaks") {
			return
		}
		st.mu.Lock()
		mode := str(data["mode"])
		if mode == "coffee" || mode == "lunch" {
			left, _ := num(data["left"])
			if left < 0 {
				left = 0
			}
			total, _ := num(data["total"])
			st.Brk = &Break{Mode: mode, Running: truthy(data["running"]), Done: truthy(data["done"]),
				Total: total, Left: left, At: clip(str(data["at"]), 5), End: now() + float64(left)/1000}
		} else {
			st.Brk = nil
		}
		st.mu.Unlock()
		send(w, http.StatusNoContent, nil, "", true)
	case "/__poll/start":
		if !only("can start a poll") {
			return
		}
		q := clip(strings.TrimSpace(str(data["q"])), 300)
		var opts []string
		if list, ok := data["opts"].([]any); ok {
			for _, o := range list {
				if t := clip(strings.TrimSpace(str(o)), 120); t != "" && len(opts) < 8 {
					opts = append(opts, t)
				}
			}
		}
		secs, ok := num(data["seconds"])
		if !ok || secs == 0 {
			secs = 30
		}
		secs = max(5, min(600, secs))
		if q == "" || len(opts) < 2 {
			send(w, http.StatusBadRequest, []byte("a question and at least two answers"), "", true)
			return
		}
		st.mu.Lock()
		if st.Poll.Q != "" && len(st.Poll.Votes) > 0 {
			st.Polls = append([]PollRecord{record(st.Poll)}, st.Polls...)
			if len(st.Polls) > 50 {
				st.Polls = st.Polls[:50]
			}
		}
		id := st.Poll.ID
		for _, h := range st.Polls {
			id = max(id, h.ID)
		}
		st.Poll = Poll{ID: id + 1, Q: q, Opts: opts, Ends: now() + float64(secs), Votes: map[string]int{}}
		st.savePolls()
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, s.pollView(true), true)
	case "/__poll/stop":
		if !only("can stop a poll") {
			return
		}
		st.mu.Lock()
		st.Poll.Ends = min(st.Poll.Ends, now())
		st.savePolls()
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, s.pollView(true), true)
	case "/__ask": // a question from the room
		txt := clip(strings.TrimSpace(reSpaces.ReplaceAllString(str(data["text"]), " ")), 400)
		name := clip(strings.TrimSpace(reSpaces.ReplaceAllString(str(data["name"]), " ")), 40)
		if txt == "" {
			send(w, http.StatusBadRequest, []byte(`{"ok": false}`), "application/json", false)
			return
		}
		who := viewer
		if who == "" {
			who = "presenter"
		}
		st.mu.Lock()
		if !local && time.Since(st.lastQ[who]) < 10*time.Second { // one question per 10 s per person
			st.mu.Unlock()
			send(w, http.StatusTooManyRequests, []byte(`{"ok": false, "wait": true}`), "application/json", false)
			return
		}
		st.lastQ[who] = time.Now()
		id := 0
		for _, q := range st.Qs {
			id = max(id, q.ID)
		}
		st.Qs = append(st.Qs, Question{ID: id + 1, Text: txt, Name: name, Slide: st.Live, At: time.Now().Format("15:04")})
		if len(st.Qs) > 200 {
			st.Qs = st.Qs[len(st.Qs)-200:]
		}
		st.saveQs()
		st.mu.Unlock()
		send(w, http.StatusOK, []byte(`{"ok": true}`), "application/json", false)
	case "/__questions/act": // the presenter reads, shows, answers, deletes
		if !only("manages questions") {
			return
		}
		act := str(data["act"])
		qid, _ := num(data["id"])
		st.mu.Lock()
		idx := -1
		for i := range st.Qs {
			if st.Qs[i].ID == qid {
				idx = i
			}
		}
		switch {
		case act == "seen_all":
			for i := range st.Qs {
				st.Qs[i].Seen = true
			}
		case act == "show" && idx >= 0:
			st.Qs[idx].Seen = true
			st.Show = qid
		case act == "hide":
			st.Show = 0
		case (act == "done" || act == "undone") && idx >= 0:
			st.Qs[idx].Done = act == "done"
			if st.Qs[idx].Done && st.Show == qid {
				st.Show = 0
			}
		case act == "delete" && idx >= 0:
			st.Qs = append(st.Qs[:idx], st.Qs[idx+1:]...)
			if st.Show == qid {
				st.Show = 0
			}
		case act == "clear":
			st.Qs, st.Show = nil, 0
		}
		st.saveQs()
		body := s.questionsLocked()
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, body, true)
	case "/__poll/clear": // the presenter decides when results go away
		if !only("clears polls") {
			return
		}
		st.mu.Lock()
		if truthy(data["all"]) {
			st.Polls = nil
			st.Poll = Poll{ID: st.Poll.ID, Votes: map[string]int{}}
		} else {
			pid, ok := num(data["id"])
			if !ok {
				pid = -1
			}
			if pid == st.Poll.ID {
				st.Poll = Poll{ID: st.Poll.ID, Votes: map[string]int{}}
			}
			kept := st.Polls[:0]
			for _, h := range st.Polls {
				if h.ID != pid {
					kept = append(kept, h)
				}
			}
			st.Polls = kept
		}
		st.savePolls()
		st.mu.Unlock()
		send(w, http.StatusOK, []byte(`{"ok": true}`), "application/json", true)
	case "/__poll/vote":
		voter := clip(str(data["voter"]), 64)
		pid, ok1 := num(data["id"])
		choice, ok2 := num(data["choice"])
		if !ok1 || !ok2 {
			text(w, http.StatusBadRequest, "bad vote")
			return
		}
		st.mu.Lock()
		ok := voter != "" && pid == st.Poll.ID && now() < st.Poll.Ends && choice >= 0 && choice < len(st.Poll.Opts)
		if ok {
			// One vote per browser that joined the session (its viewer cookie, so a whole
			// office behind one address still gets one vote each); the last answer before
			// the end counts. Browsers on this computer use their own id, for rehearsals.
			key := viewer
			if local {
				key = voter
			}
			st.Poll.Votes[key] = choice
			st.savePolls()
		}
		st.mu.Unlock()
		code := http.StatusOK
		if !ok {
			code = http.StatusConflict
		}
		sendJSON(w, code, map[string]bool{"ok": ok}, false)
	default:
		text(w, http.StatusNotFound, "not here")
	}
}

// call with mu held
func (s *Server) questionsLocked() map[string]any {
	items := s.st.Qs
	if items == nil {
		items = []Question{}
	}
	var showing any
	if s.st.Show != 0 {
		showing = s.st.Show
	}
	return map[string]any{"items": items, "showing": showing}
}

var (
	reSlide  = regexp.MustCompile(`^/(\d{1,4})$`)
	reSearch = regexp.MustCompile(`^/media/search-([\w-]+)\.json$`)
	reExID   = regexp.MustCompile(`^[a-z0-9-]{1,80}$`)
)

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	q := r.URL.Query()
	st := s.st
	st.mu.Lock()
	live := st.Live
	st.mu.Unlock()
	local := isLocal(r)

	switch path {
	case "/__session": // for the presenter's QR code ("j" in the deck)
		if !local {
			text(w, http.StatusForbidden, "not here")
			return
		}
		st.mu.Lock()
		viewers := 0
		for _, t := range st.seen {
			if time.Since(t) < 15*time.Second {
				viewers++
			}
		}
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, map[string]any{"code": st.Code, "url": s.JoinURL(), "base": s.Base(), "viewers": viewers}, true)
		return
	case "/join":
		s.join(w, r)
		return
	case "/favicon.ico":
		text(w, http.StatusNotFound, "")
		return
	}
	if !local && s.viewer(r) == "" {
		// Not in this session (yet): the join page, whatever was asked for.
		if strings.HasPrefix(path, "/__") {
			text(w, http.StatusForbidden, "join the session first")
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.RawQuery = ""
		s.join(w, r2)
		return
	}

	only := func() bool {
		if !local {
			text(w, http.StatusForbidden, "only the presenter's computer can do that")
		}
		return local
	}
	switch path {
	case "/__live/set":
		if !only() {
			return
		}
		if n, err := strconv.Atoi(q.Get("n")); err == nil && n >= 0 && n < 10000 {
			st.mu.Lock()
			if !st.Held { // held: the presenter browses, the room stays
				st.Live = n
				st.Ex = nil // a new slide page means no exercise is running
			}
			st.saveSession()
			st.mu.Unlock()
		}
		send(w, http.StatusNoContent, nil, "", true)
		return
	case "/__theme/set": // "w" in the deck: the Halloween edition, for the room too
		if !only() {
			return
		}
		st.mu.Lock()
		st.Theme = ""
		if q.Get("name") == "halloween" {
			st.Theme = "halloween"
		}
		st.mu.Unlock()
		send(w, http.StatusNoContent, nil, "", true)
		return
	case "/__live/hold": // "l" in the deck: hold the room on its slide, or release it
		if !only() {
			return
		}
		st.mu.Lock()
		st.Held = q.Get("on") == "1"
		if n, err := strconv.Atoi(q.Get("n")); !st.Held && err == nil && n >= 0 {
			st.Live = n // released: the room comes to the presenter's slide
			st.Ex = nil
		}
		body := map[string]any{"n": st.Live, "held": st.Held}
		st.saveSession()
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, body, true)
		return
	case "/__exercise/set": // the presenter opened or closed an exercise ("x" in the deck)
		if !only() {
			return
		}
		st.mu.Lock()
		if id := q.Get("id"); reExID.MatchString(id) {
			step, _ := strconv.Atoi(q.Get("step"))
			st.Ex = &Exercise{ID: id, Step: max(0, min(999, step))}
		} else {
			st.Ex = nil
		}
		st.mu.Unlock()
		send(w, http.StatusNoContent, nil, "", true)
		return
	case "/__questions": // the presenter's inbox
		if !only() {
			return
		}
		st.mu.Lock()
		body := s.questionsLocked()
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, body, true)
		return
	case "/__poll/history": // the presenter's list of polls, newest first
		if !only() {
			return
		}
		st.mu.Lock()
		rows := []PollRecord{}
		if st.Poll.Q != "" {
			cur := record(st.Poll)
			cur.Current, cur.Open = true, now() < st.Poll.Ends
			rows = append(rows, cur)
		}
		rows = append(rows, st.Polls...)
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, rows, true)
		return
	case "/__poll":
		sendJSON(w, http.StatusOK, s.pollView(local), true)
		return
	case "/__live":
		st.mu.Lock()
		var brk map[string]any
		if b := st.Brk; b != nil {
			left := b.Left
			if b.Running { // what is left now, so a phone with a wrong clock still counts down right
				left = max(0, int((b.End-now())*1000))
			}
			brk = map[string]any{"mode": b.Mode, "running": b.Running, "done": b.Done, "total": b.Total, "left": left, "at": b.At}
		}
		var shown any
		for _, x := range st.Qs {
			if st.Show != 0 && x.ID == st.Show {
				shown = map[string]any{"text": x.Text, "name": x.Name, "slide": x.Slide}
			}
		}
		body := map[string]any{"n": live, "ex": st.Ex, "held": st.Held, "brk": brk, "theme": st.Theme, "question": shown}
		st.mu.Unlock()
		sendJSON(w, http.StatusOK, body, true)
		return
	case "/media/exercises.json": // the exercise catalog, without the speaker notes
		rows := []map[string]any{}
		for _, e := range s.src.Catalog() {
			data, _ := json.Marshal(e)
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			delete(m, "notes")
			rows = append(rows, m)
		}
		sendJSON(w, http.StatusOK, rows, false)
		return
	case "/":
		http.Redirect(w, r, "/"+strconv.Itoa(live), http.StatusFound)
		return
	}

	if m := reSlide.FindStringSubmatch(path); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n > live {
			http.Redirect(w, r, "/"+strconv.Itoa(live), http.StatusFound)
			return
		}
		page, ok := s.src.Page(n)
		if !ok {
			text(w, http.StatusNotFound, "not here")
			return
		}
		send(w, http.StatusOK, []byte(audienceHTML(page, n, live)), "text/html; charset=utf-8", false)
		return
	}
	if m := reSearch.FindStringSubmatch(path); m != nil {
		// the search index, without slides after the live one and without speaker notes
		type row struct {
			N int    `json:"n"`
			K string `json:"k"`
			T string `json:"t"`
			X string `json:"x"`
			M int    `json:"m,omitempty"`
		}
		rows := []row{}
		if m[1] == s.o.Deck {
			for _, r := range s.src.Index() {
				if r.N <= live {
					rows = append(rows, row{r.N, r.K, r.T, r.X, r.M})
				}
			}
		}
		sendJSON(w, http.StatusOK, rows, false)
		return
	}
	if strings.Contains(path, "..") || strings.HasSuffix(path, "notes.html") || strings.HasSuffix(path, "print.html") ||
		!(path == "/style.css" || strings.HasPrefix(path, "/js/") || strings.HasPrefix(path, "/fonts/") ||
			strings.HasPrefix(path, "/images/") || strings.HasPrefix(path, "/media/")) {
		text(w, http.StatusNotFound, "not here")
		return
	}
	s.src.Static().ServeHTTP(w, r)
}

var (
	reTermDock  = regexp.MustCompile(`(?s)<div class="term-dock">.*?</web-term></div>`)
	reWebTerm   = regexp.MustCompile(`(?s)<web-term\b.*?</web-term>`)
	reNotes     = regexp.MustCompile(`(?s)<speaker-notes>.*?</speaker-notes>`)
	reLiveScr   = regexp.MustCompile(`<script src="/livereload\.js[^"]*"></script>`)
	reNextConst = regexp.MustCompile(`const NextURL = '[^']*';`)
	reNavNext   = regexp.MustCompile(`(<nav-arrows[^>]*\snext=")[^"]*(")`)
)

// audienceHTML strips a presenter's page down to what the room may see.
func audienceHTML(page string, n, live int) string {
	page = reTermDock.ReplaceAllString(page, "")
	page = reWebTerm.ReplaceAllString(page, "")
	page = strings.ReplaceAll(page, " has-term", "")
	page = reNotes.ReplaceAllString(page, "")
	page = reLiveScr.ReplaceAllString(page, "")
	if n >= live { // no "next" on the live slide
		self := "/" + strconv.Itoa(n)
		page = reNextConst.ReplaceAllLiteralString(page, "const NextURL = '"+self+"';")
		page = reNavNext.ReplaceAllString(page, "${1}"+self+"${2}")
	}
	boot := fmt.Sprintf("<script>window.RESPAWN_AUDIENCE = { live: %d };</script>", live)
	return strings.Replace(page, "</head>", boot+"</head>", 1)
}
