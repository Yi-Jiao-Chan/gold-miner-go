package render

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"goldminer/internal/flash"
	"goldminer/internal/i18n"
	"hash/fnv"
	"image/color"
)

// Label draws extension UI text using the same Retina-aware stage transform.
func (r *Renderer) Label(dst *ebiten.Image, s string, x, y, w, h, size float64, c color.NRGBA, align int) {
	hash := fnv.New32a()
	fmt.Fprintf(hash, "%g:%g:%g:%v:%d", w, h, size, c, align)
	t := &flash.Text{Font: i18n.Font, Height: size, Bounds: flash.Rect{W: w, H: h}, Color: [4]uint8{c.R, c.G, c.B, c.A}, Align: align, Multiline: true, WordWrap: true}
	m := Viewport(dst.Bounds().Dx(), dst.Bounds().Dy()).Mul(flash.Matrix{A: 1, D: 1, TX: x, TY: y})
	r.drawTextMode(dst, -int(hash.Sum32())-10000, t, s, m, flash.NoColor(), labelText)
}
