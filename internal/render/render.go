// Package render draws the extracted SWF display list with Ebitengine.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/colorm"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"goldminer/internal/flash"
	"goldminer/internal/i18n"
)

type Renderer struct {
	Files      fs.FS
	Images     map[string]*ebiten.Image
	Fonts      map[int]*opentype.Font
	Faces      map[string]font.Face
	TextImages map[string]*ebiten.Image
	Mask       *ebiten.Shader
	Chinese    map[int]i18n.Static
}

func New(files fs.FS, movie *flash.Movie) (*Renderer, error) {
	r := &Renderer{Files: files, Images: map[string]*ebiten.Image{}, Fonts: map[int]*opentype.Font{}, Faces: map[string]font.Face{}, TextImages: map[string]*ebiten.Image{}}
	b, err := fs.ReadFile(files, "chinese.json")
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &r.Chinese); err != nil {
		return nil, err
	}
	b, err = fs.ReadFile(files, "fonts/Chinese.ttf")
	if err != nil {
		return nil, err
	}
	r.Fonts[i18n.Font], err = opentype.Parse(b)
	if err != nil {
		return nil, err
	}
	for id, path := range movie.Fonts {
		b, err := fs.ReadFile(files, path)
		if err != nil {
			return nil, err
		}
		f, err := opentype.Parse(b)
		if err != nil {
			return nil, fmt.Errorf("font %d: %w", id, err)
		}
		r.Fonts[id] = f
	}
	shader, err := ebiten.NewShader([]byte(`//kage:unit pixels
package main
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
 return imageSrc0At(srcPos) * imageSrc1At(srcPos).a
}
`))
	if err != nil {
		return nil, err
	}
	r.Mask = shader
	for _, d := range movie.Characters {
		if d.Asset != nil {
			if _, err := r.image(d.Asset.Path); err != nil {
				return nil, err
			}
		}
		for _, a := range d.Morph {
			if _, err := r.image(a.Path); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}
func (r *Renderer) image(path string) (*ebiten.Image, error) {
	if p := r.Images[path]; p != nil {
		return p, nil
	}
	b, err := fs.ReadFile(r.Files, path)
	if err != nil {
		return nil, err
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	p := ebiten.NewImageFromImage(im)
	r.Images[path] = p
	return p, nil
}
func combine(p, c flash.ColorTransform) flash.ColorTransform {
	var n flash.ColorTransform
	for i := 0; i < 4; i++ {
		n.Mul[i] = p.Mul[i] * c.Mul[i]
		n.Add[i] = p.Mul[i]*c.Add[i] + p.Add[i]
	}
	return n
}
func geo(m flash.Matrix) ebiten.GeoM {
	var g ebiten.GeoM
	g.SetElement(0, 0, m.A)
	g.SetElement(0, 1, m.C)
	g.SetElement(0, 2, m.TX)
	g.SetElement(1, 0, m.B)
	g.SetElement(1, 1, m.D)
	g.SetElement(1, 2, m.TY)
	return g
}
func draw(dst, im *ebiten.Image, m flash.Matrix, c flash.ColorTransform) {
	var cm colorm.ColorM
	for i := 0; i < 4; i++ {
		cm.SetElement(i, i, c.Mul[i])
		cm.SetElement(i, 4, c.Add[i])
	}
	colorm.DrawImage(dst, im, cm, &colorm.DrawImageOptions{GeoM: geo(m), Filter: ebiten.FilterLinear})
}
func (r *Renderer) Draw(screen *ebiten.Image, rt *flash.Runtime) {
	screen.Fill(color.Black)
	r.node(screen, rt, rt.Root, Viewport(screen.Bounds().Dx(), screen.Bounds().Dy()), flash.NoColor())
}

// Viewport fits the original stage into the physical screen without stretching.
// The same transform is inverted for pointer input.
func Viewport(width, height int) flash.Matrix {
	s := math.Min(float64(width)/550, float64(height)/400)
	return flash.Matrix{A: s, D: s, TX: (float64(width) - 550*s) / 2, TY: (float64(height) - 400*s) / 2}
}
func (r *Renderer) node(dst *ebiten.Image, rt *flash.Runtime, n *flash.Node, parent flash.Matrix, pc flash.ColorTransform) {
	if n == nil || !n.Visible {
		return
	}
	m := parent.Mul(n.Matrix)
	c := combine(pc, n.Color)
	d := n.Def
	switch d.Kind {
	case "image":
		if zh, ok := r.Chinese[d.ID]; ok && zh.Text != zh.Source {
			t := &flash.Text{Font: i18n.Font, Height: zh.Height * 0.8, Bounds: d.Asset.Bounds, Color: zh.Color, Align: zh.Align, Multiline: true, WordWrap: strings.Contains(zh.Text, "\n")}
			r.drawTextMode(dst, d.ID, t, zh.Text, m, c, staticText)
		} else {
			r.asset(dst, *d.Asset, m, c)
		}
	case "morph":
		closest := math.MaxInt
		var chosen flash.Asset
		for ratio, a := range d.Morph {
			delta := int(math.Abs(float64(ratio - n.Ratio)))
			if delta < closest {
				closest = delta
				chosen = a
			}
		}
		if chosen.Path != "" {
			r.asset(dst, chosen, m, c)
		}
	case "text":
		t := d.Text
		value := rt.Get(n.Parent, t.Variable)
		s := t.Initial
		if value != nil {
			s = flash.String(value)
		}
		s = i18n.Dynamic(s)
		if strings.ContainsFunc(s, func(c rune) bool { return unicode.Is(unicode.Han, c) }) {
			copy := *t
			copy.Font = i18n.Font
			copy.Height *= 0.8
			t = &copy
		}
		r.drawText(dst, d.ID, t, s, m, c)
	case "sprite", "button":
		depths := make([]int, 0, len(n.Children))
		for k := range n.Children {
			depths = append(depths, k)
		}
		sort.Ints(depths)
		for i := 0; i < len(depths); i++ {
			child := n.Children[depths[i]]
			if child.ClipDepth > depths[i] {
				// Flash clipDepth masks the following siblings, never the mask itself.
				mask := ebiten.NewImage(dst.Bounds().Dx(), dst.Bounds().Dy())
				layer := ebiten.NewImage(dst.Bounds().Dx(), dst.Bounds().Dy())
				r.node(mask, rt, child, m, c)
				for i+1 < len(depths) && depths[i+1] <= child.ClipDepth {
					i++
					r.node(layer, rt, n.Children[depths[i]], m, c)
				}
				opts := &ebiten.DrawRectShaderOptions{}
				opts.Images[0] = layer
				opts.Images[1] = mask
				dst.DrawRectShader(dst.Bounds().Dx(), dst.Bounds().Dy(), r.Mask, opts)
				layer.Deallocate()
				mask.Deallocate()
			} else {
				r.node(dst, rt, child, m, c)
			}
		}
	}
}

func (r *Renderer) drawText(dst *ebiten.Image, id int, t *flash.Text, s string, m flash.Matrix, c flash.ColorTransform) {
	r.drawTextMode(dst, id, t, s, m, c, dynamicText)
}
func (r *Renderer) drawTextMode(dst *ebiten.Image, id int, t *flash.Text, s string, m flash.Matrix, c flash.ColorTransform, mode textMode) {
	scale := math.Max(1, math.Min(8, math.Ceil(math.Max(math.Hypot(m.A, m.B), math.Hypot(m.C, m.D)))))
	if im := r.textImage(id, t, s, scale, mode); im != nil {
		r.assetImage(dst, im, flash.Asset{Bounds: t.Bounds, Scale: scale}, m, c)
	}
}
func (r *Renderer) asset(dst *ebiten.Image, a flash.Asset, m flash.Matrix, c flash.ColorTransform) {
	im, _ := r.image(a.Path)
	if im != nil {
		r.assetImage(dst, im, a, m, c)
	}
}
func (r *Renderer) assetImage(dst, im *ebiten.Image, a flash.Asset, m flash.Matrix, c flash.ColorTransform) {
	local := flash.Matrix{A: 1 / a.Scale, D: 1 / a.Scale, TX: a.Bounds.X, TY: a.Bounds.Y}
	draw(dst, im, m.Mul(local), c)
}
func (r *Renderer) text(id int, t *flash.Text, s string, scale float64) *ebiten.Image {
	return r.textImage(id, t, s, scale, dynamicText)
}
func (r *Renderer) textImage(id int, t *flash.Text, s string, scale float64, mode textMode) *ebiten.Image {
	key := fmt.Sprintf("%d:%g:%d:%#v:%s", id, scale, mode, *t, s)
	if im := r.TextImages[key]; im != nil {
		return im
	}
	if t.Font == i18n.Font {
		rgba := r.chineseRaster(t, s, scale, mode)
		if rgba == nil {
			return nil
		}
		return r.cacheText(key, rgba)
	}
	fk := fmt.Sprintf("%d:%g:%g", t.Font, t.Height, scale)
	face := r.Faces[fk]
	if face == nil {
		f := r.Fonts[t.Font]
		if f == nil {
			return nil
		}
		var err error
		face, err = opentype.NewFace(f, &opentype.FaceOptions{Size: t.Height * scale, DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			return nil
		}
		r.Faces[fk] = face
	}
	w, h := int(math.Ceil(t.Bounds.W*scale)), int(math.Ceil(t.Bounds.H*scale))
	if w < 1 || h < 1 {
		return nil
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	dr := font.Drawer{Dst: rgba, Src: image.NewUniform(color.NRGBA{R: t.Color[0], G: t.Color[1], B: t.Color[2], A: t.Color[3]}), Face: face}
	usable := (t.Bounds.W - 4 - t.MarginLeft - t.MarginRight) * scale
	lines := strings.Split(s, "\n")
	if t.WordWrap {
		lines = nil
		for _, para := range strings.Split(s, "\n") {
			line := ""
			words := strings.Fields(para)
			chinese := strings.ContainsFunc(para, func(c rune) bool { return unicode.Is(unicode.Han, c) })
			if chinese {
				words = nil
				for _, c := range para {
					words = append(words, string(c))
				}
			}
			for _, word := range words {
				test := word
				if line != "" {
					if chinese {
						test = line + word
					} else {
						test = line + " " + word
					}
				}
				if float64(font.MeasureString(face, test))/64 > usable && line != "" {
					lines = append(lines, line)
					line = word
				} else {
					line = test
				}
			}
			lines = append(lines, line)
		}
	}
	baseline := 2*scale + float64(face.Metrics().Ascent)/64
	lineHeight := float64(face.Metrics().Height)/64 + t.Leading*scale

	for _, line := range lines {
		if baseline-float64(face.Metrics().Ascent)/64 >= float64(h) {
			break
		}
		advance := float64(font.MeasureString(face, line)) / 64
		x := (2 + t.MarginLeft) * scale
		if t.Align == 1 {
			x += usable - advance
		} else if t.Align == 2 {
			x += (usable - advance) / 2
		}
		dr.Dot = fixed.Point26_6{X: fixed.Int26_6(math.Round(x * 64)), Y: fixed.Int26_6(math.Round(baseline * 64))}
		dr.DrawString(line)
		baseline += lineHeight
		if !t.Multiline {
			break
		}
	}
	return r.cacheText(key, rgba)
}
func (r *Renderer) cacheText(key string, rgba *image.RGBA) *ebiten.Image {
	im := ebiten.NewImageFromImage(rgba)
	// A long run changes score/time every frame. Keep a bounded text cache.
	if len(r.TextImages) > 512 {
		for k, p := range r.TextImages {
			p.Deallocate()
			delete(r.TextImages, k)
		}
	}
	r.TextImages[key] = im
	return im
}
