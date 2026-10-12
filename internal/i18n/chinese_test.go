package i18n

import (
	"encoding/json"
	"goldminer/assets"
	"goldminer/internal/flash"
	"io/fs"
	"strings"
	"testing"
	"unicode"
)

func TestAllOriginalTextAssetsHaveChinesePresentation(t *testing.T) {
	b, e := fs.ReadFile(assets.Files, "chinese.json")
	if e != nil {
		t.Fatal(e)
	}
	var zh map[int]Static
	if e = json.Unmarshal(b, &zh); e != nil {
		t.Fatal(e)
	}
	b, e = fs.ReadFile(assets.Files, "movie.json")
	if e != nil {
		t.Fatal(e)
	}
	var m flash.Movie
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	count := 0
	for id, d := range m.Characters {
		if d.Asset == nil || !strings.HasPrefix(d.Asset.Path, "texts/") {
			continue
		}
		tr, ok := zh[id]
		if !ok || tr.Text == "" {
			t.Fatalf("text asset %d untranslated", id)
		}
		if len([]rune(tr.Source)) > 1 && !strings.ContainsFunc(tr.Text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			t.Fatalf("text %d remains English", id)
		}
		count++
	}
	if count < 50 {
		t.Fatalf("covered only %d static texts", count)
	}
	if zh[6].Text != "黄金矿工" {
		t.Fatal("graphic title untranslated")
	}
	for _, source := range []string{"After you have grabbed onto something", "Strength drink.", "Lucky Clover.", "Rock Collectors book.", "Diamond Polish.", "You scored 650 Points at Gold Miner!"} {
		text := Dynamic(source)
		if text == source || !strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			t.Fatalf("dynamic text untranslated: %s", source)
		}
	}
}
