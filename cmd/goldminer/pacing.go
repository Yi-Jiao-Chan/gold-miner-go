package main

// Poll inputs often while keeping the archived movie at exactly 18 frames/sec.
// Integer accumulation avoids time drift and never advances the movie on input.
const inputTPS = 120

type frameClock struct{ phase int }

func (c *frameClock) advance() bool {
	c.phase += 18
	if c.phase < inputTPS {
		return false
	}
	c.phase -= inputTPS
	return true
}
