// Package audiofx applies this SWF's SOUNDINFO metadata to decoded stereo PCM.
package audiofx

import (
	"encoding/binary"
	"goldminer/internal/flash"
	"math"
)

// Prepare uses signed 16-bit, stereo, 44.1 kHz PCM, matching the native player.
func Prepare(pcm []byte, s flash.SoundVariant) []byte {
	start, end := s.InPoint, len(pcm)/4
	if s.OutPoint >= 0 && s.OutPoint < end {
		end = s.OutPoint
	}
	if start < 0 {
		start = 0
	}
	if start > end {
		start = end
	}
	part := append([]byte(nil), pcm[start*4:end*4]...)
	if len(s.Envelope) > 0 {
		for i := 0; i < len(part)/4; i++ {
			// Envelope time is absolute within the original sound, before trimming.
			pos := i + start
			left, right := gain(s.Envelope, pos)
			for channel, level := range []float64{left, right} {
				at := i*4 + channel*2
				v := int16(binary.LittleEndian.Uint16(part[at:]))
				scaled := int(math.Round(float64(v) * level))
				if scaled > 32767 {
					scaled = 32767
				}
				if scaled < -32768 {
					scaled = -32768
				}
				binary.LittleEndian.PutUint16(part[at:], uint16(int16(scaled)))
			}
		}
	}
	loops := s.Loops
	if loops < 1 {
		loops = 1
	}
	out := make([]byte, 0, len(part)*loops)
	for i := 0; i < loops; i++ {
		out = append(out, part...)
	}
	return out
}
func gain(es []flash.SoundEnvelope, pos int) (float64, float64) {
	a, b := es[0], es[0]
	for _, e := range es {
		if e.Position <= pos {
			a = e
			b = e
		} else {
			b = e
			break
		}
	}
	t := 0.
	if b.Position > a.Position {
		t = float64(pos-a.Position) / float64(b.Position-a.Position)
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return (float64(a.Left)*(1-t) + float64(b.Left)*t) / 32768, (float64(a.Right)*(1-t) + float64(b.Right)*t) / 32768
}
