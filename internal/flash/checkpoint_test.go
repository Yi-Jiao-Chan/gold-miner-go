package flash

import (
	"bytes"
	"testing"
)

func TestCheckpointRestoresShopWithoutRerollAndFutureRandomness(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	ticks(t, r, 40)
	r.Set(r.Root, "score", float64(10000))
	timer := findCharacter(r.Root, 197)
	r.Set(timer, "time", float64(1))
	ticks(t, r, 220)
	if r.Root.Frame != 130 {
		t.Fatal("not in shop")
	}
	if err := r.Pointer(57, 257, false); err != nil {
		t.Fatal(err)
	}
	if err := r.Pointer(57, 257, true); err != nil {
		t.Fatal(err)
	}
	if err := r.Pointer(57, 257, false); err != nil {
		t.Fatal(err)
	}
	data, err := r.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(r.Movie, data)
	if err != nil {
		t.Fatal(err)
	}
	if number(restored.Get(restored.Root, "score")) != number(r.Get(r.Root, "score")) {
		t.Fatal("score lost")
	}
	for _, id := range []int{90, 96, 102, 107, 111} {
		a, b := findCharacter(r.Root, id), findCharacter(restored.Root, id)
		if a.Frame != b.Frame || a.Playing != b.Playing || r.Get(a, "price") != restored.Get(b, "price") {
			t.Fatalf("shop rerolled %d", id)
		}
	}
	for i := 0; i < 100; i++ {
		if r.rng.IntN(100000) != restored.rng.IntN(100000) {
			t.Fatal("random sequence changed")
		}
	}
	ticks(t, r, 200)
	ticks(t, restored, 200)
	a, _ := r.Checkpoint()
	b, _ := restored.Checkpoint()
	if !bytes.Equal(a, b) {
		t.Fatal("restored shop state diverged")
	}
}

func TestCheckpointRoundTripMovingCoopAndRejectCorruption(t *testing.T) {
	r := archivedRuntime(t)
	if err := r.SetPlayers(2); err != nil {
		t.Fatal(err)
	}
	startGame(t, r)
	ticks(t, r, 40)
	if err := r.PlayerKey(2, 15); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 12)
	// An explicit clip reference tests identity in addition to coordinates/state.
	r.Set(r.Root, "savedclip", r.Get(r.Root, "C2"))
	data, err := r.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(r.Movie, data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Players() != 2 || restored.Get(restored.Root, "savedclip") != restored.Get(restored.Root, "C2") {
		t.Fatal("player or node identity lost")
	}
	for i := 0; i < 100; i++ {
		ticks(t, r, 1)
		ticks(t, restored, 1)
		a, _ := r.Checkpoint()
		b, _ := restored.Checkpoint()
		if !bytes.Equal(a, b) {
			t.Fatalf("moving coop diverged at %d", i)
		}
	}
	for _, bad := range [][]byte{[]byte("broken"), []byte(`{"version":9}`), bytes.Replace(data, []byte(`"Parent":0`), []byte(`"Parent":1`), 1)} {
		if _, err := Restore(r.Movie, bad); err == nil {
			t.Fatal("corrupt checkpoint accepted")
		}
	}
}

func TestLegacyCoopCheckpointUpdatesSpacingWithoutResettingClaws(t *testing.T) {
	r := archivedRuntime(t)
	if err := r.SetPlayers(2); err != nil {
		t.Fatal(err)
	}
	startGame(t, r)
	ticks(t, r, 40)
	if err := r.PlayerKey(1, 15); err != nil {
		t.Fatal(err)
	}
	if err := r.PlayerKey(2, 15); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 12)
	expected, _ := r.Checkpoint()
	for _, n := range r.Root.Children {
		player := coopPlayer(n.Name)
		if player == 0 {
			continue
		}
		delta := 200 - coopLeftAnchor
		if player == 2 {
			delta = 350 - coopRightAnchor
		}
		n.Matrix.TX += delta
		n.placement.Matrix.TX += delta
	}
	legacy, err := r.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(r.Movie, legacy)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := restored.Checkpoint()
	if !bytes.Equal(expected, actual) {
		t.Fatal("old save migration reset more than root spacing")
	}
	current, err := Restore(r.Movie, expected)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		ticks(t, current, 1)
		ticks(t, restored, 1)
		a, _ := current.Checkpoint()
		b, _ := restored.Checkpoint()
		if !bytes.Equal(a, b) {
			t.Fatalf("migrated moving claw diverged at tick %d", i)
		}
	}
}
