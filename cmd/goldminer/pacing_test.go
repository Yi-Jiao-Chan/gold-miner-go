package main

import "testing"

func TestMoviePacingKeepsOriginalRate(t *testing.T) {
	var c frameClock
	ticks, last := 0, 0
	for update := 1; update <= inputTPS*60; update++ {
		if c.advance() {
			ticks++
			gap := update - last
			if gap != 6 && gap != 7 {
				t.Fatalf("uneven movie pacing: gap=%d", gap)
			}
			last = update
		}
		if update%inputTPS == 0 && ticks != update/inputTPS*18 {
			t.Fatalf("movie accelerated or slowed: updates=%d ticks=%d", update, ticks)
		}
	}
}
