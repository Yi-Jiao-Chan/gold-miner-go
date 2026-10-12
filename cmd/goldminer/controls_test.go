package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"goldminer/internal/flash"
	"testing"
)

func TestKeyboardFollowsLeftAndRightMiners(t *testing.T) {
	for _, tc := range []struct {
		name         string
		players      int
		key          ebiten.Key
		player, code int
	}{
		{"solo down", 1, ebiten.KeyArrowDown, 1, 15},
		{"solo up", 1, ebiten.KeyArrowUp, 1, 14},
		{"solo ignores S", 1, ebiten.KeyS, 0, 0},
		{"solo ignores W", 1, ebiten.KeyW, 0, 0},
		{"coop left S", 2, ebiten.KeyS, 1, 15},
		{"coop left W", 2, ebiten.KeyW, 1, 14},
		{"coop right down", 2, ebiten.KeyArrowDown, 2, 15},
		{"coop right up", 2, ebiten.KeyArrowUp, 2, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := pressedPlayerKeys(tc.players, func(k ebiten.Key) bool { return k == tc.key })
			if tc.player == 0 {
				if len(keys) != 0 {
					t.Fatal("solo accepted co-op key")
				}
				return
			}
			if len(keys) != 1 || keys[0] != (playerKey{tc.player, tc.code}) {
				t.Fatalf("keys=%v", keys)
			}
		})
	}
	for _, key := range []ebiten.Key{ebiten.KeyS, ebiten.KeyArrowDown} {
		g := testSession(t)
		click(t, g, "coop")
		updates(t, g, 300)
		if e := g.step(controls{Keys: pressedPlayerKeys(2, func(k ebiten.Key) bool { return k == key })}); e != nil {
			t.Fatal(e)
		}
		left := g.rt.Get(g.rt.Root, "C.cl").(*flash.Node)
		right := g.rt.Get(g.rt.Root, "C2.cl").(*flash.Node)
		if left.Playing != (key == ebiten.KeyS) || right.Playing != (key == ebiten.KeyArrowDown) {
			t.Fatalf("key %v changed wrong miner", key)
		}
	}
}
