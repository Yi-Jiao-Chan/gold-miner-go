package main

import "github.com/hajimehoshi/ebiten/v2"

// Player one is always the left miner. Solo uses arrows; co-op follows the
// physical keyboard layout with W/S on the left and arrows on the right.
func pressedPlayerKeys(players int, pressed func(ebiten.Key) bool) []playerKey {
	arrowPlayer := 1
	if players == 2 {
		arrowPlayer = 2
	}
	keys := []struct {
		key          ebiten.Key
		player, code int
	}{
		{ebiten.KeyArrowLeft, arrowPlayer, 1},
		{ebiten.KeyArrowRight, arrowPlayer, 2},
		{ebiten.KeyArrowUp, arrowPlayer, 14},
		{ebiten.KeyArrowDown, arrowPlayer, 15},
	}
	if players == 2 {
		keys = append(keys, struct {
			key          ebiten.Key
			player, code int
		}{ebiten.KeyW, 1, 14}, struct {
			key          ebiten.Key
			player, code int
		}{ebiten.KeyS, 1, 15})
	}
	var out []playerKey
	for _, k := range keys {
		if pressed(k.key) {
			out = append(out, playerKey{k.player, k.code})
		}
	}
	return out
}
