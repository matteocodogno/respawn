package audience

import (
	"io"
	"log"
)

// quietLog drops http.Server's own messages: visitors who leave mid-answer are normal
// (phones, the tunnel).
func quietLog() *log.Logger { return log.New(io.Discard, "", 0) }

const joinPage = `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>{title} · join</title>
<style>
:root{color-scheme:dark}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:#16091f;color:#f3e6c4;
 font:600 18px/1.4 system-ui,-apple-system,Segoe UI,sans-serif;padding:16px;box-sizing:border-box}
form{width:min(420px,100%);display:flex;flex-direction:column;gap:14px;background:#24112f;
 border:3px solid #000;box-shadow:8px 8px 0 #000;padding:28px;box-sizing:border-box}
h1{margin:0;font:900 40px/1 Impact,Haettenschweiler,sans-serif;letter-spacing:.02em;color:#ffd600;
 text-shadow:3px 3px 0 #ff2e88;text-transform:uppercase}
input{font:800 32px/1 ui-monospace,Menlo,monospace;letter-spacing:.3em;text-transform:uppercase;
 text-align:center;padding:12px;border:3px solid #000;background:#f3e6c4;color:#000;width:100%;box-sizing:border-box}
button{font:800 18px/1 system-ui,sans-serif;letter-spacing:.1em;text-transform:uppercase;padding:14px;
 border:3px solid #000;background:#ffd600;color:#000;box-shadow:4px 4px 0 #000;cursor:pointer}
p{margin:0;opacity:.8}.err{color:#ff6b6b;opacity:1}
</style></head><body><form method="get" action="/join">
<h1>{title} live</h1><p>Scan the QR code on the screen, or type the session code.</p>
<input name="s" maxlength="12" autocomplete="off" autocapitalize="characters" spellcheck="false" autofocus
 aria-label="Session code" placeholder="CODE">{err}<button type="submit">Join</button></form></body></html>`
