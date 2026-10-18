package flash

import (
	"bytes"
	"encoding/json"
	"testing"
)

func legacyCoopScoreCheckpoint(t *testing.T, data []byte) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document["player_scores"]; !ok {
		t.Fatal("checkpoint did not contain player_scores")
	}
	delete(document, "player_scores")
	legacy, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return legacy
}

func buyCoopScoreProduct(t *testing.T, r *Runtime, item coopScoreShopItem) float64 {
	t.Helper()
	product := findCharacter(r.Root, item.id)
	buy := findCharacter(r.Root, item.button)
	if product == nil || buy == nil || product.Frame != 2 {
		t.Fatalf("product %d unavailable", item.id)
	}
	price := number(r.Get(product, "price"))
	coopScorePointerTicks(t, r, item.x, item.y, false)
	if r.hover != buy {
		t.Fatalf("pointer missed product %d", item.id)
	}
	coopScorePointerTicks(t, r, item.x, item.y, true)
	coopScorePointerTicks(t, r, item.x, item.y, false)
	return price
}

func TestCooperativeScoreCheckpointRoundTripAndContinuation(t *testing.T) {
	r := coopScoreShopRuntime(t)
	item := coopScoreShopItems()[0]
	product := findCharacter(r.Root, item.id)
	if product == nil {
		t.Fatal("first product unavailable")
	}
	r.Set(product, "price", float64(101))
	price := buyCoopScoreProduct(t, r, item)
	want := [2]float64{9999 - price/2, 1 - price/2}
	assertCoopScores(t, r, want)

	data, err := r.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(r.Movie, data)
	if err != nil {
		t.Fatal(err)
	}
	assertCoopScores(t, restored, want)

	if err := restored.Goto(restored.Root, "next", true); err != nil {
		t.Fatal(err)
	}
	for i := 0; restored.Get(restored.Root, "C2") == nil && i < 100; i++ {
		ticks(t, restored, 1)
	}
	if restored.Get(restored.Root, "C2") == nil {
		t.Fatal("next level did not restore cooperative miners")
	}
	coopNode(t, restored, "C").Playing = false
	coopNode(t, restored, "C2").Playing = false
	b2 := coopNode(t, restored, "bonus2")
	before := restored.PlayerScores()
	restored.Set(b2, "bscore", float64(7))
	if err := restored.Goto(b2, 2, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, restored, 32)
	assertCoopScores(t, restored, [2]float64{before[0], before[1] + 7})

	data, err = restored.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	continued, err := Restore(restored.Movie, data)
	if err != nil {
		t.Fatal(err)
	}
	assertCoopScores(t, continued, [2]float64{before[0], before[1] + 7})
}

func TestLegacyCooperativeScoreCheckpointSplitsSharedScoreOnce(t *testing.T) {
	t.Run("level", func(t *testing.T) {
		r := cooperativeRuntime(t)
		coopNode(t, r, "C").Playing = false
		coopNode(t, r, "C2").Playing = false
		coopScoreBonus(t, r, 1, float64(41))
		coopScoreBonus(t, r, 2, float64(72))
		assertCoopScores(t, r, [2]float64{41, 72})
		data, err := r.Checkpoint()
		if err != nil {
			t.Fatal(err)
		}
		legacy := legacyCoopScoreCheckpoint(t, data)
		restored, err := Restore(r.Movie, legacy)
		if err != nil {
			t.Fatal(err)
		}
		assertCoopScores(t, restored, [2]float64{56.5, 56.5})

		b2 := coopNode(t, restored, "bonus2")
		restored.Set(b2, "bscore", float64(7))
		if err := restored.Goto(b2, 2, true); err != nil {
			t.Fatal(err)
		}
		ticks(t, restored, 32)
		assertCoopScores(t, restored, [2]float64{56.5, 63.5})
		data, err = restored.Checkpoint()
		if err != nil {
			t.Fatal(err)
		}
		restoredAgain, err := Restore(restored.Movie, data)
		if err != nil {
			t.Fatal(err)
		}
		assertCoopScores(t, restoredAgain, [2]float64{56.5, 63.5})
	})

	t.Run("shop", func(t *testing.T) {
		r := coopScoreShopRuntime(t)
		r.Set(r.Root, "score", float64(10001))
		assertCoopScores(t, r, [2]float64{9999.5, 1.5})
		data, err := r.Checkpoint()
		if err != nil {
			t.Fatal(err)
		}
		restored, err := Restore(r.Movie, legacyCoopScoreCheckpoint(t, data))
		if err != nil {
			t.Fatal(err)
		}
		assertCoopScores(t, restored, [2]float64{5000.5, 5000.5})
		if restored.Root.Frame != 130 {
			t.Fatalf("legacy shop restore frame=%d", restored.Root.Frame)
		}
	})
}

func TestCooperativeScoreCheckpointBytesRemainStableAfterRestore(t *testing.T) {
	r := coopScoreShopRuntime(t)
	data, err := r.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(r.Movie, data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, got) {
		t.Fatal("score checkpoint changed after immediate restore")
	}
}
