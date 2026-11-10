// Package assets embeds mypack's brand artwork (the official app icon,
// cropped to its squircle and downscaled).
package assets

import _ "embed"

//go:embed brand-256.png
var Brand256 []byte

//go:embed brand-64.png
var Brand64 []byte
