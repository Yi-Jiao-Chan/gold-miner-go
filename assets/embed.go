// Package assets contains the original artwork, fonts, audio and timelines.
package assets

import "embed"

//go:embed movie.json chinese.json shapes texts images fonts sounds morphshapes
var Files embed.FS
