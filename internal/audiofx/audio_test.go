package audiofx

import (
	"encoding/binary"
	"goldminer/internal/flash"
	"testing"
)

func TestOriginalTrimRepeatAndEnvelope(t *testing.T) {
	pcm := make([]byte, 40)
	for i := 0; i < 20; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], 1000)
	}
	s := flash.SoundVariant{InPoint: 2, OutPoint: 5, Loops: 3, Envelope: []flash.SoundEnvelope{{Position: 0, Left: 16384, Right: 32768}}}
	out := Prepare(pcm, s)
	if len(out) != 36 {
		t.Fatalf("trimmed 3 sample frames repeated 3 times: %d bytes", len(out))
	}
	for i := 0; i < len(out); i += 4 {
		if binary.LittleEndian.Uint16(out[i:]) != 500 || binary.LittleEndian.Uint16(out[i+2:]) != 1000 {
			t.Fatal("channel envelope changed")
		}
	}
}
