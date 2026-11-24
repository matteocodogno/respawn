// Package respawn holds the files built into the binary: the runtime every deck shares
// (styles, fonts, scripts, the notes and print pages). A presentation overrides any of them
// with a file at the same path in its own assets folder.
package respawn

import "embed"

// Assets is the built-in runtime, under "assets/".
//
//go:embed all:assets
var Assets embed.FS

// Starter is what `respawn new` copies: the showcase presentation, under "examples/showcase/".
//
//go:embed examples/showcase
var Starter embed.FS
