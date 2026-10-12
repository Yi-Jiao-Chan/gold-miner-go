// Package flash implements the subset of Flash 5 timelines and AVM1 used by
// the archived Gold Miner SWF. It does not depend on a Flash plugin/player.
package flash

// Matrix transforms local pixel coordinates to parent pixel coordinates.
// x' = A*x + C*y + TX; y' = B*x + D*y + TY.
type Matrix struct{ A, B, C, D, TX, TY float64 }

func Identity() Matrix { return Matrix{A: 1, D: 1} }
func (m Matrix) Mul(n Matrix) Matrix {
	return Matrix{A: m.A*n.A + m.C*n.B, B: m.B*n.A + m.D*n.B, C: m.A*n.C + m.C*n.D, D: m.B*n.C + m.D*n.D, TX: m.A*n.TX + m.C*n.TY + m.TX, TY: m.B*n.TX + m.D*n.TY + m.TY}
}
func (m Matrix) Point(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.TX, m.B*x + m.D*y + m.TY
}

type Rect struct{ X, Y, W, H float64 }
type ColorTransform struct {
	Mul [4]float64
	Add [4]float64
}

func NoColor() ColorTransform { return ColorTransform{Mul: [4]float64{1, 1, 1, 1}} }

type Asset struct {
	Path   string  `json:"path"`
	Bounds Rect    `json:"bounds"`
	Scale  float64 `json:"scale"`
}
type Text struct {
	Variable    string   `json:"variable"`
	Initial     string   `json:"initial"`
	Font        int      `json:"font"`
	Height      float64  `json:"height"`
	Color       [4]uint8 `json:"color"`
	Align       int      `json:"align"`
	Bounds      Rect     `json:"bounds"`
	Multiline   bool     `json:"multiline"`
	WordWrap    bool     `json:"word_wrap"`
	Leading     float64  `json:"leading"`
	MarginLeft  float64  `json:"margin_left"`
	MarginRight float64  `json:"margin_right"`
	Indent      float64  `json:"indent"`
}
type ClipAction struct {
	Event string `json:"event"`
	Code  []byte `json:"code"`
}
type ButtonAction struct {
	Event string `json:"event"`
	Key   int    `json:"key"`
	Code  []byte `json:"code"`
}
type Placement struct {
	Depth     int            `json:"depth"`
	Character int            `json:"character"`
	Name      string         `json:"name"`
	Matrix    Matrix         `json:"matrix"`
	Color     ColorTransform `json:"color"`
	Ratio     int            `json:"ratio"`
	ClipDepth int            `json:"clip_depth"`
	// Born records the frame where this instance was first placed. It distinguishes
	// identical character IDs that must reset when seeking between level layouts.
	Born        int          `json:"born"`
	ClipActions []ClipAction `json:"clip_actions,omitempty"`
}
type Frame struct {
	Display []Placement `json:"display"`
	Actions [][]byte    `json:"actions,omitempty"`
	Sounds  []int       `json:"sounds,omitempty"`
}
type Character struct {
	ID     int            `json:"id"`
	Kind   string         `json:"kind"`
	Frames []Frame        `json:"frames,omitempty"`
	Labels map[string]int `json:"labels,omitempty"`
	Asset  *Asset         `json:"asset,omitempty"`
	Morph  map[int]Asset  `json:"morph,omitempty"`
	Text   *Text          `json:"text,omitempty"`
	// Button states: 1 up, 2 over, 4 down, 8 hit test. Placements per state.
	States        map[int][]Placement `json:"states,omitempty"`
	ButtonActions []ButtonAction      `json:"button_actions,omitempty"`
	ButtonSounds  map[string]int      `json:"button_sounds,omitempty"`
	Bounds        Rect                `json:"bounds"`
}
type Movie struct {
	SHA256        string `json:"sha256"`
	Width, Height int
	FPS           float64              `json:"fps"`
	Characters    map[int]*Character   `json:"characters"`
	Fonts         map[int]string       `json:"fonts"`
	Sounds        map[int]string       `json:"sounds"`
	SoundVariants map[int]SoundVariant `json:"sound_variants,omitempty"`
	Opcodes       []int                `json:"opcodes"`
}

// SoundVariant retains Flash SOUNDINFO trims, repetition and channel envelope.
// Positions are in 44.1 kHz sample frames, regardless of source WAV sample rate.
type SoundEnvelope struct {
	Position int `json:"position"`
	Left     int `json:"left"`
	Right    int `json:"right"`
}
type SoundVariant struct {
	Source     int             `json:"source"`
	InPoint    int             `json:"in_point"`
	OutPoint   int             `json:"out_point"`
	Loops      int             `json:"loops"`
	NoMultiple bool            `json:"no_multiple"`
	Envelope   []SoundEnvelope `json:"envelope,omitempty"`
}
