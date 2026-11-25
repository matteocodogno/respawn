package deck

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Quiz is one multiple-choice question of a live quiz: the presenter starts it, the room
// answers on their phones against the clock. The answer never reaches the slides' HTML:
// it lives only on the server.
type Quiz struct {
	ID       string   `json:"id"`
	Slide    int      `json:"slide"`
	Question string   `json:"q"`       // HTML
	Options  []string `json:"options"` // HTML
	Answer   int      `json:"answer"`  // index into Options
	Explain  string   `json:"explain,omitempty"`
	Time     int      `json:"time"`   // seconds to answer
	Points   int      `json:"points"` // for a right answer given at once; half of it at the last second
}

// QuizConfig is "quiz:" in respawn.yaml: the rules of the game.
type QuizConfig struct {
	Time    int  `yaml:"time"`    // seconds per question (default 15)
	Points  int  `yaml:"points"`  // a right answer at once (default 1000)
	Penalty int  `yaml:"penalty"` // lost for a wrong answer, or no answer without jokers (default points/4)
	Jokers  *int `yaml:"jokers"`  // questions a player may skip without losing points (default 2)
}

// FreeSkips is the number of jokers of each player.
func (q QuizConfig) FreeSkips() int {
	if q.Jokers == nil {
		return 2
	}
	return max(0, *q.Jokers)
}

func (q *QuizConfig) defaults() {
	if q.Time <= 0 {
		q.Time = 15
	}
	if q.Points <= 0 {
		q.Points = 1000
	}
	if q.Penalty <= 0 {
		q.Penalty = q.Points / 4
	}
}

var reQuizID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,60}$`)

// quizComponent renders "::: quiz": the question and its options for the slide, and the
// whole quiz (answer included) in a <quiz-key> that Build takes out of the HTML.
func quizComponent(p map[string]string, content string) (string, error) {
	var y struct {
		Question any   `yaml:"question"`
		Options  []any `yaml:"options"`
		Answer   any   `yaml:"answer"`
		Explain  any   `yaml:"explain"`
	}
	if err := yamlOf(content, &y); err != nil {
		return "", err
	}
	q := Quiz{ID: p["id"]}
	if q.ID != "" && !reQuizID.MatchString(q.ID) {
		return "", fmt.Errorf("id %q: lowercase letters, digits and dashes", q.ID)
	}
	q.Question = inline(y.Question).Render()
	if strings.TrimSpace(q.Question) == "" {
		return "", fmt.Errorf("question: missing")
	}
	if len(y.Options) < 2 || len(y.Options) > 8 {
		return "", fmt.Errorf("options: between 2 and 8, found %d", len(y.Options))
	}
	for _, o := range y.Options {
		q.Options = append(q.Options, inline(o).Render())
	}
	q.Answer = -1
	switch a := y.Answer.(type) {
	case int:
		q.Answer = a - 1
	case string:
		a = strings.TrimSpace(a)
		if n, err := strconv.Atoi(a); err == nil {
			q.Answer = n - 1
		} else if len(a) == 1 {
			q.Answer = int(strings.ToUpper(a)[0]) - 'A'
		}
	}
	if q.Answer < 0 || q.Answer >= len(q.Options) {
		return "", fmt.Errorf("answer: the right option, as a letter (B) or its number from 1 (2); got %v", y.Answer)
	}
	q.Explain = inline(y.Explain).Render()
	for k, dst := range map[string]*int{"time": &q.Time, "points": &q.Points} {
		if v := p[k]; v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return "", fmt.Errorf("%s=%q: a positive number", k, v)
			}
			*dst = n
		}
	}
	key, _ := json.Marshal(q)
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="quiz" data-quiz="%s">`, q.ID)
	b.WriteString(`<div class="quiz-q">` + q.Question + `</div>`)
	b.WriteString(`<quiz-options><ol class="quiz-opts">`)
	for i, o := range q.Options {
		fmt.Fprintf(&b, `<li data-i="%d"><span class="ql">%c</span><span class="qt">%s</span><span class="qn"></span></li>`, i, 'A'+i, o)
	}
	b.WriteString(`</ol></quiz-options>`)
	b.WriteString(`<quiz-key hidden>` + html.EscapeString(string(key)) + `</quiz-key></div>`)
	return b.String(), nil
}

var (
	reQuizKey  = regexp.MustCompile(`(?s)<quiz-key hidden>(.*?)</quiz-key>`)
	reQuizOpts = regexp.MustCompile(`(?s)<quiz-options>.*?</quiz-options>`)
	reQuizSlot = regexp.MustCompile(`data-quiz=""`)
)

// takeQuizzes removes the quiz keys from a slide and returns its quizzes, with ids
// (base-1, base-2... when the slide does not name them) and the rules of respawn.yaml.
func takeQuizzes(h string, slide int, base string, rules QuizConfig) (string, []*Quiz, error) {
	var out []*Quiz
	var err error
	k := 0
	h = reQuizKey.ReplaceAllStringFunc(h, func(m string) string {
		var q Quiz
		raw := html.UnescapeString(reQuizKey.FindStringSubmatch(m)[1])
		if e := json.Unmarshal([]byte(raw), &q); e != nil {
			err = e
			return ""
		}
		k++
		if q.ID == "" {
			q.ID = fmt.Sprintf("%s-%d", base, k)
		}
		if q.Time == 0 {
			q.Time = rules.Time
		}
		if q.Points == 0 {
			q.Points = rules.Points
		}
		q.Slide = slide
		out = append(out, &q)
		return ""
	})
	i := 0
	h = reQuizSlot.ReplaceAllStringFunc(h, func(string) string {
		id := ""
		for ; i < len(out); i++ {
			if strings.HasPrefix(out[i].ID, base+"-") {
				id = out[i].ID
				i++
				break
			}
		}
		return `data-quiz="` + id + `"`
	})
	if len(out) > 0 {
		var notes strings.Builder
		for _, q := range out {
			fmt.Fprintf(&notes, "<br><b>Quiz answer: %c.</b> %s", 'A'+q.Answer, q.Options[q.Answer])
			if q.Explain != "" {
				notes.WriteString(" · " + q.Explain)
			}
		}
		if strings.Contains(h, "</speaker-notes>") {
			h = strings.Replace(h, "</speaker-notes>", notes.String()+"</speaker-notes>", 1)
		} else {
			h += "\n<speaker-notes>" + strings.TrimPrefix(notes.String(), "<br>") + "</speaker-notes>"
		}
	}
	return h, out, err
}
