package render

import (
	"encoding/json"
	"fmt"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"goldminer/internal/i18n"
	"image"
	"io/fs"
	"math"
	"strings"
	"testing"

	"goldminer/assets"
	"goldminer/internal/flash"
)

func TestViewportKeepsPointerOnStageAfterResize(t *testing.T) {
	for _, size := range [][2]int{{550, 400}, {1100, 800}, {2200, 1600}, {2000, 1000}, {900, 1200}} {
		v := Viewport(size[0], size[1])
		for _, p := range [][2]float64{{0, 0}, {550, 400}, {275, 200}, {83.5, 291.25}} {
			x, y := v.Point(p[0], p[1])
			if math.Abs((x-v.TX)/v.A-p[0]) > 1e-9 || math.Abs((y-v.TY)/v.D-p[1]) > 1e-9 {
				t.Fatalf("pointer moved after resize to %v", size)
			}
			if x < 0 || y < 0 || x > float64(size[0]) || y > float64(size[1]) {
				t.Fatalf("stage cropped after resize to %v", size)
			}
		}
	}
}

func TestTextRasterMatchesRetinaResolution(t *testing.T) {
	b, err := fs.ReadFile(assets.Files, "movie.json")
	if err != nil {
		t.Fatal(err)
	}
	var m flash.Movie
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	r, err := New(assets.Files, &m)
	if err != nil {
		t.Fatal(err)
	}
	for id, d := range m.Characters {
		if d.Text == nil || r.Fonts[d.Text.Font] == nil {
			continue
		}
		lo := r.text(id, d.Text, "650", 1)
		hi := r.text(id, d.Text, "650", 4)
		if lo == nil || hi == nil || lo == hi {
			t.Fatal("Retina text reused low resolution raster")
		}
		if hi.Bounds().Dx() != int(math.Ceil(d.Text.Bounds.W*4)) || hi.Bounds().Dy() != int(math.Ceil(d.Text.Bounds.H*4)) {
			t.Fatal("Retina text raster has wrong dimensions")
		}
		return
	}
	t.Fatal("no embedded dynamic font tested")
}

func chineseTestRenderer(t *testing.T) (*Renderer, *flash.Movie) {
	t.Helper()
	b, err := fs.ReadFile(assets.Files, "movie.json")
	if err != nil {
		t.Fatal(err)
	}
	var m flash.Movie
	if err = json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	r, err := New(assets.Files, &m)
	if err != nil {
		t.Fatal(err)
	}
	return r, &m
}

// Measure and rasterize outside the clipping rectangle as well: an in-bounds
// bitmap alone would conceal missing glyph pixels at its edges.
func assertChineseFits(t *testing.T, r *Renderer, field *flash.Text, s string, scale float64, mode textMode) {
	t.Helper()
	layout := r.layoutChinese(field, s, scale, mode)
	if layout.face == nil {
		t.Fatal("no Chinese layout")
	}
	var rendered strings.Builder
	pad := 100
	w, h := int(math.Ceil(field.Bounds.W*scale)), int(math.Ceil(field.Bounds.H*scale))
	full := image.NewRGBA(image.Rect(0, 0, w+2*pad, h+2*pad))
	d := font.Drawer{Dst: full, Face: layout.face, Src: image.White}
	for _, line := range layout.lines {
		rendered.WriteString(line.text)
		if !line.ink.Empty() {
			minX, maxX := line.x+float64(line.ink.Min.X)/64, line.x+float64(line.ink.Max.X)/64
			minY, maxY := line.y+float64(line.ink.Min.Y)/64, line.y+float64(line.ink.Max.Y)/64
			if minX < 0 || minY < 0 || maxX > float64(w) || maxY > float64(h) {
				t.Fatalf("glyphs outside authored bounds: (%g,%g)-(%g,%g), %dx%d", minX, minY, maxX, maxY, w, h)
			}
		}
		d.Dot = fixed.Point26_6{X: fixed.Int26_6(math.Round((line.x + float64(pad)) * 64)), Y: fixed.Int26_6(math.Round((line.y + float64(pad)) * 64))}
		d.DrawString(line.text)
	}
	if rendered.String() != strings.ReplaceAll(s, "\n", "") {
		t.Fatal("layout dropped translated text")
	}
	n := 0
	for y := 0; y < full.Bounds().Dy(); y++ {
		for x := 0; x < full.Bounds().Dx(); x++ {
			if full.RGBAAt(x, y).A == 0 {
				continue
			}
			n++
			if x < pad || y < pad || x >= pad+w || y >= pad+h {
				t.Fatal("raster clips visible glyph pixels")
			}
		}
	}
	if n == 0 {
		t.Fatal("empty Chinese raster")
	}
}

func TestChineseTranslationsStayInsideOriginalInkBounds(t *testing.T) {
	r, m := chineseTestRenderer(t)
	for id, translation := range r.Chinese {
		asset := m.Characters[id].Asset
		if translation.Source == translation.Text {
			continue
		}
		field := &flash.Text{Font: i18n.Font, Height: translation.Height * 0.8, Bounds: asset.Bounds, Color: translation.Color, Align: translation.Align, Multiline: true, WordWrap: strings.Contains(translation.Text, "\n")}
		for _, scale := range []float64{1, 4} {
			t.Run(fmt.Sprintf("%d/%gx", id, scale), func(t *testing.T) { assertChineseFits(t, r, field, translation.Text, scale, staticText) })
		}
	}
	for _, id := range []int{188, 189, 192, 195} {
		if r.Chinese[id].Align != 0 {
			t.Fatalf("HUD label %d lost its left anchor", id)
		}
	}
}

func TestChineseShopDescriptionsFitAtNativeAndRetinaSizes(t *testing.T) {
	r, m := chineseTestRenderer(t)
	field := *m.Characters[84].Text
	field.Font = i18n.Font
	field.Height *= 0.8
	for _, source := range []string{"After you have grabbed something", "Strength drink.", "Lucky Clover.", "Rock Collectors book.", "Diamond Polish."} {
		for _, scale := range []float64{1, 4} {
			t.Run(fmt.Sprintf("%s/%gx", source, scale), func(t *testing.T) { assertChineseFits(t, r, &field, i18n.Dynamic(source), scale, dynamicText) })
		}
	}
}

func TestNativeLabelCentersVisibleGlyphsAndCacheIncludesStyle(t *testing.T) {
	r, _ := chineseTestRenderer(t)
	field := &flash.Text{Font: i18n.Font, Height: 12, Bounds: flash.Rect{W: 80, H: 30}, Color: [4]uint8{255, 255, 255, 255}, Align: 2, Multiline: true, WordWrap: true}
	for _, scale := range []float64{1, 4} {
		layout := r.layoutChinese(field, "下一关", scale, labelText)
		if len(layout.lines) != 1 {
			t.Fatal("unexpected label wrap")
		}
		line := layout.lines[0]
		cx := line.x + float64(line.ink.Min.X+line.ink.Max.X)/128
		cy := line.y + float64(line.ink.Min.Y+line.ink.Max.Y)/128
		if math.Abs(cx-40*scale) > 0.02 || math.Abs(cy-15*scale) > 0.02 {
			t.Fatal("label glyphs not centered")
		}
		assertChineseFits(t, r, field, "下一关", scale, labelText)
	}
	first := r.textImage(1, field, "下一关", 1, labelText)
	changed := *field
	changed.Align = 0
	second := r.textImage(1, &changed, "下一关", 1, labelText)
	if first == second {
		t.Fatal("text cache ignored alignment")
	}
	changed.Color[0] = 0
	if second == r.textImage(1, &changed, "下一关", 1, labelText) {
		t.Fatal("text cache ignored color")
	}
	if first == r.textImage(1, field, "下一关", 1, dynamicText) {
		t.Fatal("text cache ignored vertical layout mode")
	}
}
