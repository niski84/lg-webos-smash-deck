package web

import "embed"

// FS holds all files under web/lgdeck/ embedded into the binary at compile time.
//go:embed lgdeck
var FS embed.FS
