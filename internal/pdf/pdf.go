// Package pdf prints a range of slides to a 16:9 PDF with headless Chrome: one slide per
// page, 1600x900, no margins, no header or footer, no print dialog.
//
// Chrome opens /media/print.html?headless=1 on the presenter's own server, which loads the
// slides in thumbnail mode and sets window.__printReady when every one is ready; then the
// page goes straight to Page.printToPDF.
package pdf

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/matteocodogno/respawn/internal/deck"
)

var candidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
	`C:\Program Files\Google\Chrome\Application\chrome.exe`,
	`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
}

// FindChrome returns a Chrome-like browser: RESPAWN_CHROME first, then the usual places.
func FindChrome() string {
	if env := os.Getenv("RESPAWN_CHROME"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	home, _ := os.UserHomeDir()
	for _, c := range candidates {
		for _, p := range []string{c, filepath.Join(home, c)} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave-browser", "microsoft-edge"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, pattern := range []string{
		filepath.Join(home, "Library/Caches/ms-playwright/chromium-*/chrome-mac*/*.app/Contents/MacOS/*"),
		filepath.Join(home, ".cache/ms-playwright/chromium-*/chrome-linux*/chrome"),
		"/opt/pw-browsers/chromium-*/chrome-linux*/chrome",
	} {
		if found, _ := filepath.Glob(pattern); len(found) > 0 {
			return found[len(found)-1]
		}
	}
	return ""
}

// Request is one export.
type Request struct {
	Base     string // the presenter's server, e.g. http://127.0.0.1:8889
	Deck     string
	From, To int
	Theme    string
	Log      func(format string, args ...any)
}

// Render returns the PDF bytes of slides From..To.
func Render(ctx context.Context, r Request) ([]byte, error) {
	chrome := FindChrome()
	if chrome == "" {
		return nil, errors.New("no Chrome, Chromium, Brave or Edge found: install one, or set RESPAWN_CHROME")
	}
	if r.From > r.To {
		r.From, r.To = r.To, r.From
	}
	q := url.Values{}
	q.Set("from", fmt.Sprint(r.From))
	q.Set("to", fmt.Sprint(r.To))
	q.Set("deck", r.Deck)
	q.Set("headless", "1")
	if r.Theme != "" {
		q.Set("theme", r.Theme)
	}
	target := strings.TrimRight(r.Base, "/") + "/media/print.html?" + q.Encode()
	count := r.To - r.From + 1
	timeout := 40*time.Second + time.Duration(count)*1500*time.Millisecond

	profile, err := os.MkdirTemp("", "respawn-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(profile)

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chrome),
		chromedp.UserDataDir(profile), // never the user's running Chrome
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WindowSize(1600, 900),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		// a page nobody looks at must not be throttled: the slides load in iframes on timers
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("no-proxy-server", true), // the slides are on this computer
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		opts = append(opts, chromedp.NoSandbox)
	}
	actx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	cctx, cancel := chromedp.NewContext(actx)
	defer cancel()

	stage := "starting " + filepath.Base(chrome)
	fail := func(err error) ([]byte, error) {
		return nil, fmt.Errorf("%v while %s (%s; slides at %s)", err, stage, filepath.Base(chrome), r.Base)
	}
	// the first call starts the browser: no deadline on it (it would stop the browser)
	if err := chromedp.Do(cctx); err != nil {
		return fail(err)
	}
	tctx, cancelT := context.WithTimeout(cctx, timeout)
	defer cancelT()
	stage = "opening the slides"
	if err := chromedp.Do(tctx, chromedp.Navigate(target)); err != nil {
		return fail(err)
	}
	stage = "loading the slides"
	type status struct {
		Ready    bool   `json:"ready"`
		Done     int    `json:"done"`
		Href     string `json:"href"`
		Title    string `json:"title"`
		Frames   int    `json:"frames"`
		Errors   string `json:"errors"`
		HasReady bool   `json:"hasReady"`
	}
	var st status
	for {
		got, err := chromedp.Run(tctx, chromedp.Evaluate[status](`({ready: window.__printReady === true, done: window.__printProgress || 0,
			href: location.href, title: document.title, frames: document.querySelectorAll('iframe').length,
			errors: (window.__printErrors || []).join(' | '), hasReady: '__printErrors' in window})`))
		if err == nil {
			st = got
		}
		if err != nil {
			if errors.Is(tctx.Err(), context.DeadlineExceeded) {
				return fail(fmt.Errorf("took too long (%d of %d slides ready; page %q, %d frames%s)",
					st.Done, count, st.Href, st.Frames, errSuffix(st.Errors)))
			}
			return fail(err)
		}
		if st.Ready {
			break
		}
		if !st.HasReady && st.Href != "" && !strings.HasPrefix(st.Href, "about:") {
			return fail(fmt.Errorf("%s is not respawn's print page (title %q)", st.Href, st.Title))
		}
		select {
		case <-tctx.Done():
			return fail(fmt.Errorf("took too long (%d of %d slides ready; page %q, %d frames%s)",
				st.Done, count, st.Href, st.Frames, errSuffix(st.Errors)))
		case <-time.After(400 * time.Millisecond):
		}
	}
	stage = "printing"
	pdf, err := chromedp.Run(tctx, chromedp.PrintToPDF(
		chromedp.PDFPrintBackground(),
		chromedp.PDFPreferCSSPageSize(),
		chromedp.PDFMargin(0),
	))
	if err != nil {
		return fail(err)
	}
	return pdf, nil
}

func errSuffix(e string) string {
	if e == "" {
		return ""
	}
	return ", errors: " + e
}

var reModule = regexp.MustCompile(`(?i)MODULE\s*(\d+)`)
var reUnsafe = regexp.MustCompile(`[^\p{L}\p{N}_ .,&()-]+`)

// Module is a module opener of the deck: its slide number, module number and title.
type Module struct {
	N     int
	Num   string
	Title string
}

// Modules picks the module openers from the search index rows (m set).
func Modules(rows []deck.IndexRow) []Module {
	var out []Module
	for _, r := range rows {
		if r.M == 0 {
			continue
		}
		num := ""
		if m := reModule.FindStringSubmatch(r.X); m != nil {
			num = m[1]
		}
		out = append(out, Module{N: r.N, Num: num, Title: r.T})
	}
	return out
}

// Name is the file name of an export, e.g.
// "AI4Dev - Module 13 Agents Inside the Product - slides 336-374.pdf".
func Name(title string, mods []Module, a, b int) string {
	slides := fmt.Sprintf("slides %d-%d", a, b)
	if a == b {
		slides = fmt.Sprintf("slide %d", a)
	}
	at := func(n int) *Module {
		var found *Module
		for i := range mods {
			if mods[i].N <= n {
				found = &mods[i]
			}
		}
		return found
	}
	name := ""
	ma, mb := at(a), at(b)
	switch {
	case ma != nil && mb != nil && ma.N == mb.N:
		name = strings.TrimSpace("Module " + ma.Num + " " + ma.Title)
	case ma != nil || mb != nil:
		first, last := ma, mb
		if first == nil {
			first = mb
		}
		if last == nil {
			last = ma
		}
		if first.Num != last.Num {
			name = "Modules " + first.Num + "-" + last.Num
		} else {
			name = "Module " + first.Num
		}
	}
	var parts []string
	for _, p := range []string{title, name, slides} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return reUnsafe.ReplaceAllString(strings.Join(parts, " - "), "") + ".pdf"
}
