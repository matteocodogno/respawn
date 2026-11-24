// Terminals on the slides: /shell/<folder> opens a shell in a folder of the presentation,
// in a ghostty-web terminal bridged to a PTY over a WebSocket. Adapted from demoit
// (github.com/dgageot/demoit, Apache-2.0).
package terminal

import (
	"bytes"
	"embed"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

//go:embed resources/terminal.html
var terminalHTML []byte

//go:embed resources/ghostty-web
var terminalAssets embed.FS

// AssetPrefix is the versioned route of the pinned terminal emulator.
const AssetPrefix = "/terminal-assets/hvir-v0.4.0-20-demoit-render1/"

const zshShell = "zsh"

// Terminals serves the terminal routes of one presentation.
type Terminals struct {
	Root    string          // the presentation folder: /shell/<folder> is relative to it
	Bashrc  string          // optional rc file sourced by every shell
	History func() []string // commands preloaded in the shell history (up arrow)
}

// Shell redirects to the terminal page with the shell command to run.
func (t *Terminals) Shell(w http.ResponseWriter, r *http.Request) {
	folder := strings.TrimPrefix(r.URL.Path, "/shell")
	folder = strings.Trim(folder, "/")
	path := t.Root
	if folder != "." && folder != "" {
		path = filepath.Join(path, filepath.FromSlash(folder))
		if rel, err := filepath.Rel(t.Root, path); err != nil || strings.HasPrefix(rel, "..") {
			http.Error(w, "folder outside the presentation", http.StatusBadRequest)
			return
		}
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	prefill := r.URL.Query().Get("command")
	if strings.ContainsFunc(prefill, unicode.IsControl) {
		http.Error(w, "Command must not contain control characters", http.StatusBadRequest)
		return
	}

	commands, err := t.shellCommands(path, prefill)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	redirectURL := "/terminal?cmd=" + url.QueryEscape(strings.Join(commands, ";"))
	if prefill != "" {
		redirectURL += "&autofocus=1"
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// Page serves the ghostty-web terminal page.
func (t *Terminals) Page(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(terminalHTML)
}

// WebSocket bridges the terminal page to a PTY.
func (t *Terminals) WebSocket(w http.ResponseWriter, r *http.Request) {
	cmd := r.URL.Query().Get("cmd")
	if cmd == "" {
		http.Error(w, "Missing cmd parameter", http.StatusBadRequest)
		return
	}
	HandleWebSocket(w, r, cmd)
}

// Asset serves the pinned terminal emulator.
func (t *Terminals) Asset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, AssetPrefix)
	switch name {
	case "ghostty-web.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case "ghostty-vt.wasm":
		w.Header().Set("Content-Type", "application/wasm")
	default:
		http.NotFound(w, r)
		return
	}
	data, err := terminalAssets.ReadFile("resources/ghostty-web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func (t *Terminals) shellCommands(path, prefill string) ([]string, error) {
	commands := []string{"cd " + shellQuote(path) + ">/dev/null"}

	shellBin, found := os.LookupEnv("SHELL")
	if !found || shellBin == "" {
		shellBin = "bash"
	}
	bashRc := ""
	if t.Bashrc != "" {
		if _, err := os.Stat(t.Bashrc); err == nil {
			bashRc = t.Bashrc
		}
	}
	historyFile, err := t.historyFile(shellBin)
	if err != nil {
		return nil, err
	}
	if prefill != "" && filepath.Base(shellBin) != zshShell {
		// demoit needs zsh to type a command for the presenter; elsewhere it goes to history
		prefill = ""
	}
	execCmd, err := shellExecCommand(shellBin, bashRc, historyFile, prefill)
	if err != nil {
		return nil, err
	}
	return append(commands, execCmd), nil
}

// historyFile writes the presentation's history to a temp file, in zsh format for zsh.
func (t *Terminals) historyFile(shellBin string) (string, error) {
	if t.History == nil {
		return "", nil
	}
	lines := t.History()
	if len(lines) == 0 {
		return "", nil
	}
	content := []byte(strings.Join(lines, "\n") + "\n")
	if filepath.Base(shellBin) == zshShell {
		content = convertToZshHistory(content)
	}
	return writeTempFile("respawn-history-*", string(content))
}

// shellExecCommand builds the exec command for the given shell, including
// wrapper init files when needed to ensure HISTFILE survives shell startup.
func shellExecCommand(shellBin, bashRc, historyFile, prefill string) (string, error) {
	if prefill != "" && filepath.Base(shellBin) != zshShell {
		return "", fmt.Errorf("command prefill requires zsh, got %s", shellBin)
	}
	switch filepath.Base(shellBin) {
	case "bash":
		return bashExecCommand(shellBin, bashRc, historyFile)
	case zshShell:
		return zshExecCommand(shellBin, bashRc, historyFile, prefill)
	default:
		return defaultExecCommand(shellBin, bashRc, historyFile), nil
	}
}

// bashExecCommand creates a temp --rcfile that sources the user's .bashrc,
// then the presentation's .bashrc, then sets HISTFILE and reloads history.
// This ensures HISTFILE is set after the user's startup files.
func bashExecCommand(shellBin, bashRc, historyFile string) (string, error) {
	if bashRc == "" && historyFile == "" {
		return "exec " + shellBin, nil
	}

	var rc strings.Builder
	rc.WriteString("[ -f \"$HOME/.bashrc\" ] && source \"$HOME/.bashrc\"\n")
	if bashRc != "" {
		fmt.Fprintf(&rc, "source %q\n", bashRc)
	}
	if historyFile != "" {
		fmt.Fprintf(&rc, "export HISTFILE=%q\n", historyFile)
		rc.WriteString("history -r \"$HISTFILE\"\n")
	}

	rcFile, err := writeTempFile("respawn-bashrc-*", rc.String())
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("exec %s --rcfile %s", shellBin, rcFile), nil
}

// zshExecCommand creates a temp ZDOTDIR with .zshenv and .zshrc wrappers
// that source the user's real startup files, then set HISTFILE.
// This ensures HISTFILE is set after the user's startup files.
func zshExecCommand(shellBin, bashRc, historyFile, prefill string) (string, error) {
	if bashRc == "" && historyFile == "" && prefill == "" {
		return "exec " + shellBin, nil
	}

	zdotdir, err := os.MkdirTemp("", "respawn-zdotdir-*")
	if err != nil {
		return "", fmt.Errorf("unable to create temp zdotdir: %w", err)
	}

	// .zshenv wrapper: source the user's real .zshenv.
	zshenv := "[ -f \"$HOME/.zshenv\" ] && source \"$HOME/.zshenv\"\n"
	if err := os.WriteFile(filepath.Join(zdotdir, ".zshenv"), []byte(zshenv), 0o600); err != nil {
		return "", fmt.Errorf("unable to write .zshenv: %w", err)
	}

	// .zshrc wrapper: reset ZDOTDIR, source user's .zshrc, then the presentation's, then set HISTFILE.
	var zshrc strings.Builder
	zshrc.WriteString("unset ZDOTDIR\n")
	zshrc.WriteString("[ -f \"$HOME/.zshrc\" ] && source \"$HOME/.zshrc\"\n")
	if bashRc != "" {
		fmt.Fprintf(&zshrc, "source %q\n", bashRc)
	}
	if historyFile != "" {
		fmt.Fprintf(&zshrc, "export HISTFILE=%q\n", historyFile)
		zshrc.WriteString("fc -R \"$HISTFILE\"\n")
	}

	if prefill != "" {
		fmt.Fprintf(&zshrc, "print -rz -- %s\n", shellQuote(prefill))
	}

	if err := os.WriteFile(filepath.Join(zdotdir, ".zshrc"), []byte(zshrc.String()), 0o600); err != nil {
		return "", fmt.Errorf("unable to write .zshrc: %w", err)
	}

	return fmt.Sprintf("ZDOTDIR=%s exec %s", zdotdir, shellBin), nil
}

// defaultExecCommand builds a fallback exec command for unknown shells.
// HISTFILE is set as an environment variable before exec, which may be
// overridden by the shell's startup files.
func defaultExecCommand(shellBin, bashRc, historyFile string) string {
	var parts []string
	if bashRc != "" {
		parts = append(parts, "source "+bashRc)
	}
	if historyFile != "" {
		parts = append(parts, fmt.Sprintf("HISTFILE=%s exec %s", historyFile, shellBin))
	} else {
		parts = append(parts, "exec "+shellBin)
	}
	return strings.Join(parts, ";")
}

// convertToZshHistory converts bash history (one command per line) to
// zsh extended history format (`: timestamp:0;command` per line).
func convertToZshHistory(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var buf strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		fmt.Fprintf(&buf, ": 0:0;%s\n", line)
	}
	return []byte(buf.String())
}

// writeTempFile creates a temp file with the given content and returns its path.
func writeTempFile(pattern, content string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("unable to create temp file: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("unable to write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("unable to close temp file: %w", err)
	}
	return f.Name(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
