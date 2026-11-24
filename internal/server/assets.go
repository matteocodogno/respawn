package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/matteocodogno/respawn"
)

// overlay looks in the presentation's assets folder first, then in the built-in runtime.
type overlay struct {
	local fs.FS // may be nil
	embed fs.FS
}

func newOverlay(dir string) *overlay {
	sub, _ := fs.Sub(respawn.Assets, "assets")
	o := &overlay{embed: sub}
	if dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			o.local = os.DirFS(dir)
		}
	}
	return o
}

func (o *overlay) Open(name string) (fs.File, error) {
	if o.local != nil {
		f, err := o.local.Open(name)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return o.embed.Open(name)
}

// hasher gives the short content hash demoit puts on asset URLs (cache busting).
type hasher struct {
	fs    fs.FS
	cache sync.Map
	keep  bool // false in --dev: files change under us
}

func (h *hasher) hash(name string) string {
	if h.keep {
		if v, ok := h.cache.Load(name); ok {
			return v.(string)
		}
	}
	f, err := h.fs.Open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return ""
	}
	v := hex.EncodeToString(sum.Sum(nil))[:10]
	h.cache.Store(name, v)
	return v
}
