// Package tunnel runs a Cloudflare Tunnel (cloudflared) to the audience copy, so the room
// joins from any network at the presentation's public address instead of this computer's.
//
// One-time setup, outside respawn: cloudflared tunnel login; cloudflared tunnel create <name>;
// cloudflared tunnel route dns <name> <host>.
package tunnel

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Tunnel is a running cloudflared.
type Tunnel struct {
	cmd  *exec.Cmd
	conf string
	Log  string // cloudflared's own log
	done chan error
}

// Start runs the named tunnel, routing host to the local port.
func Start(name, host string, port int) (*Tunnel, error) {
	bin, err := exec.LookPath("cloudflared")
	if err != nil {
		return nil, errors.New("--tunnel needs cloudflared (brew install cloudflared) and a tunnel set up once: see respawn's README")
	}
	conf, err := os.CreateTemp("", "respawn-tunnel-*.yml")
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(conf, "ingress:\n  - hostname: %s\n    service: http://localhost:%d\n  - service: http_status:404\n", host, port)
	conf.Close()
	logPath := filepath.Join(os.TempDir(), "respawn-tunnel-"+name+".log")
	logf, err := os.Create(logPath)
	if err != nil {
		os.Remove(conf.Name())
		return nil, err
	}
	cmd := exec.Command(bin, "tunnel", "--no-autoupdate", "--config", conf.Name(), "run", name)
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		logf.Close()
		os.Remove(conf.Name())
		return nil, err
	}
	t := &Tunnel{cmd: cmd, conf: conf.Name(), Log: logPath, done: make(chan error, 1)}
	go func() { t.done <- cmd.Wait(); logf.Close() }()
	select { // a tunnel that is not set up stops at once: say so now
	case err := <-t.done:
		os.Remove(t.conf)
		return nil, fmt.Errorf("cloudflared stopped at once (%v): see %s", err, logPath)
	case <-time.After(1500 * time.Millisecond):
	}
	return t, nil
}

// Wait returns when cloudflared stops.
func (t *Tunnel) Wait() error {
	err := <-t.done
	t.done <- err
	return err
}

// Stop ends cloudflared.
func (t *Tunnel) Stop() {
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(os.Interrupt)
		select {
		case err := <-t.done:
			t.done <- err
		case <-time.After(3 * time.Second):
			_ = t.cmd.Process.Kill()
		}
	}
	os.Remove(t.conf)
}
