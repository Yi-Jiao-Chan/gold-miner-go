package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"goldminer/internal/flash"
	"goldminer/internal/i18n"
)

type textMode uint8

const (
	dynamicText textMode = iota
	staticText
	labelText
)

type chineseLine struct {
	text string
	x, y float64
	ink  fixed.Rectangle26_6
}

type chineseLayout struct {
	face  font.Face
	lines []chineseLine
}

func (r *Renderer) chineseFace(size, scale float64) font.Face {
	key := fmt.Sprintf("%d:%g:%g", i18n.Font, size, scale)
	if f := r.Faces[key]; f != nil {
		return f
	}
	f := r.Fonts[i18n.Font]
	if f == nil {
		return nil
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size * scale, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	r.Faces[key] = face
	return face
}

// Static exports use tight ink rectangles, whereas edit fields and native UI
// labels include a text-box gutter. Never expand these rectangles: their parent
// transforms and neighboring score digits were authored against their bounds.
func (r *Renderer) layoutChinese(t *flash.Text, s string, scale float64, mode textMode) chineseLayout {
	inset := 2.0
	if mode == staticText {
		inset = 0.5
	}
	left, right := (inset+t.MarginLeft)*scale, (inset+t.MarginRight)*scale
	width, height := t.Bounds.W*scale-left-right, (t.Bounds.H-2*inset)*scale
	if width <= 0 || height <= 0 || t.Height <= 0 {
		return chineseLayout{}
	}
	size := t.Height
	for attempt := 0; attempt < 48; attempt++ {
		face := r.chineseFace(size, scale)
		if face == nil {
			return chineseLayout{}
		}
		lines := chineseLines(face, s, width, t.WordWrap, t.Multiline)
		lineHeight := (size*1.3 + t.Leading) * scale
		layout := chineseLayout{face: face}
		minY, maxY, widest := math.Inf(1), math.Inf(-1), 0.0
		for i, line := range lines {
			ink, advance := font.BoundString(face, line)
			minX := math.Min(0, float64(ink.Min.X)/64)
			maxX := math.Max(float64(advance)/64, float64(ink.Max.X)/64)
			widest = math.Max(widest, maxX-minX)
			y := float64(i) * lineHeight
			if !ink.Empty() {
				minY = math.Min(minY, y+float64(ink.Min.Y)/64)
				maxY = math.Max(maxY, y+float64(ink.Max.Y)/64)
			}
			layout.lines = append(layout.lines, chineseLine{text: line, y: y, ink: ink})
		}
		if math.IsInf(minY, 1) {
			return layout
		}
		blockHeight := maxY - minY
		if widest <= width && blockHeight <= height {
			// A single UI label is centered by its visible glyphs, rather than
			// Noto's unusually tall ascent. Paragraphs retain their top anchor.
			yOffset := inset*scale - minY
			if (mode == staticText && (len(lines) == 1 || t.Align == 2)) || (mode == labelText && len(lines) == 1) {
				yOffset += (height - blockHeight) / 2
			}
			for i := range layout.lines {
				line := &layout.lines[i]
				inkWidth := float64(line.ink.Max.X-line.ink.Min.X) / 64
				line.x = left - float64(line.ink.Min.X)/64
				if t.Align == 1 {
					line.x += width - inkWidth
				} else if t.Align == 2 {
					line.x += (width - inkWidth) / 2
				}
				line.y += yOffset
			}
			return layout
		}
		// Re-wrap after shrinking, since this can reduce the line count as
		// well as glyph height. Numeric edit fields never enter this path.
		size *= math.Min(0.95, math.Min(width/math.Max(widest, 1), height/math.Max(blockHeight, 1)))
	}
	return chineseLayout{}
}

func chineseLines(face font.Face, s string, width float64, wrap, multiline bool) []string {
	paragraphs := strings.Split(s, "\n")
	if !multiline {
		paragraphs = paragraphs[:1]
	}
	var lines []string
	for _, paragraph := range paragraphs {
		if !wrap {
			lines = append(lines, paragraph)
			continue
		}
		line := ""
		for _, ch := range paragraph {
			candidate := line + string(ch)
			ink, advance := font.BoundString(face, candidate)
			w := math.Max(float64(advance)/64, float64(ink.Max.X)/64) - math.Min(0, float64(ink.Min.X)/64)
			if w > width && line != "" {
				lines = append(lines, line)
				line = string(ch)
			} else {
				line = candidate
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func (r *Renderer) chineseRaster(t *flash.Text, s string, scale float64, mode textMode) *image.RGBA {
	w, h := int(math.Ceil(t.Bounds.W*scale)), int(math.Ceil(t.Bounds.H*scale))
	if w < 1 || h < 1 {
		return nil
	}
	layout := r.layoutChinese(t, s, scale, mode)
	if layout.face == nil {
		return nil
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	d := font.Drawer{Dst: rgba, Face: layout.face, Src: image.NewUniform(color.NRGBA{R: t.Color[0], G: t.Color[1], B: t.Color[2], A: t.Color[3]})}
	for _, line := range layout.lines {
		d.Dot = fixed.Point26_6{X: fixed.Int26_6(math.Round(line.x * 64)), Y: fixed.Int26_6(math.Round(line.y * 64))}
		d.DrawString(line.text)
	}
	return rgba
}
