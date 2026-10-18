package flash

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

// Node is a live display-list instance; Vars use Flash 5's case-insensitive names.
type Node struct {
	Def              *Character
	Parent           *Node
	Children         map[int]*Node
	Name             string
	Matrix           Matrix
	Color            ColorTransform
	Ratio, ClipDepth int
	Visible          bool
	Frame            int
	Playing          bool
	ButtonState      int
	Vars             map[string]any
	placement        Placement
	revision         uint64
	initialized      bool
	player           int
}

type actionJob struct {
	node *Node
	code []byte
}

// Runtime runs one SWF frame per Tick. Input methods use SWF button key codes.
type Runtime struct {
	Movie          *Movie
	Root           *Node
	LastError      error
	BlockedURLs    []string
	rng            *rand.Rand
	pcg            *rand.PCG
	players        int
	playerContext  int
	coopBusy       [2]bool
	coopScores     [2]float64
	jobs           []actionJob
	sounds         []int
	draining       bool
	hover, pressed *Node
	pointerDown    bool
}

func New(movie *Movie, seed uint64) (*Runtime, error) {
	if movie == nil || movie.Characters[0] == nil {
		return nil, fmt.Errorf("movie has no root timeline")
	}
	pcg := rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)
	r := &Runtime{Movie: movie, pcg: pcg, rng: rand.New(pcg), players: 1}
	r.Root = r.makeNode(movie.Characters[0], nil, Placement{Matrix: Identity(), Color: NoColor()})
	if err := r.seek(r.Root, 1, true); err != nil {
		return nil, err
	}
	if err := r.drain(); err != nil {
		return nil, err
	}
	return r, nil
}
func (r *Runtime) makeNode(def *Character, parent *Node, p Placement) *Node {
	n := &Node{Def: def, Parent: parent, Children: make(map[int]*Node), Name: p.Name, Matrix: p.Matrix, Color: p.Color, Ratio: p.Ratio, ClipDepth: p.ClipDepth, Visible: true, Frame: 1, Playing: true, ButtonState: 1, Vars: make(map[string]any), placement: p}
	if parent != nil {
		n.player = parent.player
		if parent == r.Root {
			n.player = coopPlayer(p.Name)
		}
	}
	return n
}
func ordered(n *Node) []*Node {
	depths := make([]int, 0, len(n.Children))
	for d := range n.Children {
		depths = append(depths, d)
	}
	sort.Ints(depths)
	out := make([]*Node, 0, len(depths))
	for _, d := range depths {
		out = append(out, n.Children[d])
	}
	return out
}
func (r *Runtime) attached(n *Node) bool {
	if n == nil {
		return false
	}
	if n == r.Root {
		return true
	}
	if n.Parent == nil {
		return false
	}
	for _, c := range n.Parent.Children {
		if c == n {
			return r.attached(n.Parent)
		}
	}
	return false
}
func (r *Runtime) display(n *Node, ps []Placement) error {
	if n == r.Root {
		ps = r.coopPlacements(ps)
	}
	old := n.Children
	next := make(map[int]*Node, len(ps))
	created := []*Node{}
	for _, p := range ps {
		def := r.Movie.Characters[p.Character]
		if def == nil {
			return fmt.Errorf("missing character %d", p.Character)
		}
		c := old[p.Depth]
		if c == nil || c.Def.ID != p.Character || c.placement.Born != p.Born {
			c = r.makeNode(def, n, p)
			created = append(created, c)
		} else {
			// Frame snapshots repeat placements. Preserve script properties until an
			// authored placement actually changes the corresponding transform.
			if c.placement.Matrix != p.Matrix {
				c.Matrix = p.Matrix
			}
			if c.placement.Color != p.Color {
				c.Color = p.Color
			}
			c.Ratio = p.Ratio
			c.ClipDepth = p.ClipDepth
			c.Name = p.Name
			c.placement = p
		}
		next[p.Depth] = c
	}
	n.Children = next
	for _, c := range created {
		if len(c.Def.Frames) > 0 {
			if err := r.seek(c, 1, true); err != nil {
				return err
			}
		}
		if len(c.Def.States) > 0 {
			if err := r.buttonState(c, 1); err != nil {
				return err
			}
		}
		if c.Def.Text != nil && c.Def.Text.Variable != "" && r.Get(c.Parent, c.Def.Text.Variable) == nil {
			r.Set(c.Parent, c.Def.Text.Variable, c.Def.Text.Initial)
		}
		for _, a := range c.placement.ClipActions {
			if strings.EqualFold(a.Event, "load") {
				r.jobs = append(r.jobs, actionJob{c, a.Code})
			}
		}
	}
	return nil
}
func (r *Runtime) seek(n *Node, frame int, play bool) error {
	if n == nil || len(n.Def.Frames) == 0 {
		return nil
	}
	if frame < 1 {
		frame = 1
	}
	if frame > len(n.Def.Frames) {
		frame = len(n.Def.Frames)
	}
	if n.initialized && n.Frame == frame {
		n.Playing = play
		return nil
	}
	n.Frame = frame
	n.Playing = play
	n.revision++
	n.initialized = true
	f := n.Def.Frames[frame-1]
	if err := r.display(n, f.Display); err != nil {
		return err
	}
	r.sounds = append(r.sounds, f.Sounds...)
	for _, code := range f.Actions {
		r.jobs = append(r.jobs, actionJob{n, code})
	}
	return nil
}
func (r *Runtime) frame(n *Node, value any) int {
	if s, ok := value.(string); ok {
		for k, v := range n.Def.Labels {
			if strings.EqualFold(k, s) {
				return v
			}
		}
	}
	return int(number(value))
}
func (r *Runtime) Goto(n *Node, value any, play bool) error {
	if n == nil {
		return nil
	}
	if err := r.seek(n, r.frame(n, value), play); err != nil {
		return r.fail(err)
	}
	return r.drain()
}
func (r *Runtime) fail(err error) error {
	if err != nil {
		r.LastError = err
	}
	return err
}
func (r *Runtime) drain() error {
	if r.draining {
		return nil
	}
	r.draining = true
	defer func() { r.draining = false }()
	for count := 0; len(r.jobs) > 0; count++ {
		if count >= 10000 {
			return r.fail(fmt.Errorf("timeline action recursion limit"))
		}
		j := r.jobs[0]
		r.jobs = r.jobs[1:]
		if !r.attached(j.node) {
			continue
		}
		frame := j.node.Frame
		if err := r.run(j.node, j.code); err != nil {
			return r.fail(fmt.Errorf("character %d frame %d: %w", j.node.Def.ID, j.node.Frame, err))
		}
		r.coopTimerAction(j.node, frame)
	}
	return nil
}

// Tick first sends enterFrame to existing clips, then advances timelines that
// those handlers did not seek. Newly created clips begin advancing next tick.
func (r *Runtime) Tick() error {
	if r.LastError != nil {
		return r.LastError
	}
	type snap struct {
		n   *Node
		rev uint64
	}
	nodes := []snap{}
	var walk func(*Node)
	walk = func(n *Node) {
		nodes = append(nodes, snap{n, n.revision})
		for _, c := range ordered(n) {
			walk(c)
		}
	}
	walk(r.Root)
	for _, s := range nodes {
		if !r.attached(s.n) {
			continue
		}
		for _, a := range s.n.placement.ClipActions {
			if strings.EqualFold(a.Event, "enterFrame") {
				r.jobs = append(r.jobs, actionJob{s.n, a.Code})
			}
		}
	}
	if err := r.drain(); err != nil {
		return err
	}
	for _, s := range nodes {
		n := s.n
		if !r.attached(n) || !n.Playing || n.revision != s.rev || len(n.Def.Frames) == 0 {
			continue
		}
		f := n.Frame + 1
		var advance bool
		f, advance = r.coopTimerStep(n, f)
		if !advance {
			continue
		}
		if f > len(n.Def.Frames) {
			f = 1
		}
		if err := r.seek(n, f, true); err != nil {
			return r.fail(err)
		}
	}
	return r.drain()
}
func (r *Runtime) DrainSounds() []int { s := r.sounds; r.sounds = nil; return s }
func (r *Runtime) World(n *Node) Matrix {
	if n == nil {
		return Identity()
	}
	if n.Parent == nil {
		return n.Matrix
	}
	return r.World(n.Parent).Mul(n.Matrix)
}
func transformed(b Rect, m Matrix) Rect {
	xs := []float64{b.X, b.X + b.W, b.X, b.X + b.W}
	ys := []float64{b.Y, b.Y, b.Y + b.H, b.Y + b.H}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i := range xs {
		x, y := m.Point(xs[i], ys[i])
		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)
	}
	return Rect{minX, minY, maxX - minX, maxY - minY}
}
func union(a, b Rect) Rect {
	x, y := math.Min(a.X, b.X), math.Min(a.Y, b.Y)
	return Rect{x, y, math.Max(a.X+a.W, b.X+b.W) - x, math.Max(a.Y+a.H, b.Y+b.H) - y}
}

// Bounds includes invisible geometry, as MovieClip.hitTest does in Flash.
func (r *Runtime) Bounds(n *Node) Rect {
	if n == nil {
		return Rect{}
	}
	var b Rect
	have := false
	if n.Def.Asset != nil {
		b = transformed(n.Def.Asset.Bounds, r.World(n))
		have = true
	} else if n.Def.Text != nil {
		b = transformed(n.Def.Text.Bounds, r.World(n))
		have = true
	} else if n.Def.Kind != "sprite" && n.Def.Kind != "button" {
		b = transformed(n.Def.Bounds, r.World(n))
		have = true
	}
	for _, c := range ordered(n) {
		q := r.Bounds(c)
		if q.W == 0 && q.H == 0 {
			continue
		}
		if have {
			b = union(b, q)
		} else {
			b = q
			have = true
		}
	}
	return b
}
func intersects(a, b Rect) bool {
	return a.W > 0 && a.H > 0 && b.W > 0 && b.H > 0 && a.X <= b.X+b.W && b.X <= a.X+a.W && a.Y <= b.Y+b.H && b.Y <= a.Y+a.H
}
func inside(b Rect, x, y float64) bool {
	return b.W > 0 && b.H > 0 && x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H
}
func (r *Runtime) buttonState(n *Node, state int) error {
	n.ButtonState = state
	ps := n.Def.States[state]
	if len(ps) == 0 {
		ps = n.Def.States[1]
	}
	return r.display(n, ps)
}
func (r *Runtime) buttonHit(n *Node, x, y float64) bool {
	for _, p := range n.Def.States[8] {
		def := r.Movie.Characters[p.Character]
		if def == nil {
			continue
		}
		b := def.Bounds
		if def.Asset != nil {
			b = def.Asset.Bounds
		}
		if inside(transformed(b, r.World(n).Mul(p.Matrix)), x, y) {
			return true
		}
	}
	return false
}
func (r *Runtime) buttonEvent(n *Node, event string) {
	if n == nil {
		return
	}
	for name, sound := range n.Def.ButtonSounds {
		if strings.EqualFold(name, event) && sound != 0 {
			r.sounds = append(r.sounds, sound)
		}
	}
	for _, a := range n.Def.ButtonActions {
		if strings.EqualFold(a.Event, event) {
			r.jobs = append(r.jobs, actionJob{n.Parent, a.Code})
		}
	}
}
func (r *Runtime) Key(code int) error {
	return r.PlayerKey(1, code)
}
func (r *Runtime) keyForPlayer(player, code int) error {
	var walk func(*Node)
	walk = func(n *Node) {
		if !n.Visible {
			return
		}
		if r.Players() == 2 && n.player != 0 && n.player != player {
			return
		}
		for _, a := range n.Def.ButtonActions {
			if a.Key == code && code != 0 {
				r.jobs = append(r.jobs, actionJob{n.Parent, a.Code})
			}
		}
		for _, c := range ordered(n) {
			walk(c)
		}
	}
	walk(r.Root)
	return r.drain()
}
func (r *Runtime) Pointer(x, y float64, down bool) error {
	var hit *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if !n.Visible {
			return
		}
		if len(n.Def.States) > 0 && r.buttonHit(n, x, y) {
			hit = n
		}
		for _, c := range ordered(n) {
			walk(c)
		}
	}
	walk(r.Root)
	if hit != r.hover {
		r.buttonEvent(r.hover, "rollOut")
		if r.hover != nil {
			if err := r.buttonState(r.hover, 1); err != nil {
				return r.fail(err)
			}
		}
		r.hover = hit
		r.buttonEvent(hit, "rollOver")
		if hit != nil {
			if err := r.buttonState(hit, 2); err != nil {
				return r.fail(err)
			}
		}
	}
	if down && !r.pointerDown {
		r.pressed = hit
		r.buttonEvent(hit, "press")
		if hit != nil {
			if err := r.buttonState(hit, 4); err != nil {
				return r.fail(err)
			}
		}
	}
	if !down && r.pointerDown {
		if r.pressed == hit {
			r.buttonEvent(hit, "release")
		} else {
			r.buttonEvent(r.pressed, "releaseOutside")
		}
		if hit != nil {
			if err := r.buttonState(hit, 2); err != nil {
				return r.fail(err)
			}
		}
		r.pressed = nil
	}
	r.pointerDown = down
	return r.drain()
}
func (r *Runtime) Get(n *Node, path string) any {
	target, key := r.variable(n, path)
	return r.member(target, key)
}
func (r *Runtime) Set(n *Node, path string, value any) {
	target, key := r.variable(n, path)
	r.setMember(target, key, value)
}
func (n *Node) Get(name string) any {
	if n == nil {
		return nil
	}
	return n.Vars[strings.ToLower(name)]
}
func (r *Runtime) variable(n *Node, path string) (*Node, string) {
	path = strings.ReplaceAll(path, ":", ".")
	path = strings.ReplaceAll(path, "/", ".")
	if strings.HasPrefix(path, ".") {
		n = r.Root
		path = strings.TrimLeft(path, ".")
	}
	parts := strings.Split(path, ".")
	for len(parts) > 1 {
		if parts[0] != "" {
			v := r.member(n, parts[0])
			n, _ = v.(*Node)
		}
		parts = parts[1:]
	}
	return n, parts[0]
}
func (r *Runtime) resolve(n *Node, v any) *Node {
	if target, ok := v.(*Node); ok {
		return target
	}
	s := stringValue(v)
	if s == "" {
		return n
	}
	target, _ := r.Get(n, s).(*Node)
	return target
}

var propertyNames = []string{"_x", "_y", "_xscale", "_yscale", "_currentframe", "_totalframes", "_alpha", "_visible", "_width", "_height", "_rotation", "_target", "_framesloaded", "_name"}

func (r *Runtime) member(n *Node, name string) any {
	if n == nil {
		return nil
	}
	key := strings.ToLower(name)
	if r.Players() == 2 && n == r.Root && r.playerContext == 2 {
		if key == "c" || key == "miner" || key == "bonus" {
			key += "2"
		}
	}
	switch key {
	case "this":
		return n
	case "_root", "_level0":
		return r.Root
	case "_parent":
		return n.Parent
	case "_x":
		return n.Matrix.TX
	case "_y":
		return n.Matrix.TY
	case "_xscale":
		return math.Hypot(n.Matrix.A, n.Matrix.B) * 100
	case "_yscale":
		return math.Hypot(n.Matrix.C, n.Matrix.D) * 100
	case "_rotation":
		return math.Atan2(n.Matrix.B, n.Matrix.A) * 180 / math.Pi
	case "_visible":
		return n.Visible
	case "_alpha":
		return n.Color.Mul[3] * 100
	case "_currentframe":
		return float64(n.Frame)
	case "_totalframes", "_framesloaded":
		return float64(len(n.Def.Frames))
	case "_name":
		return n.Name
	case "_width":
		return r.Bounds(n).W
	case "_height":
		return r.Bounds(n).H
	}
	if v, ok := n.Vars[key]; ok {
		return v
	}
	for _, c := range ordered(n) {
		if strings.EqualFold(c.Name, key) && c.Name != "" {
			return c
		}
	}
	return nil
}
func (r *Runtime) setMember(n *Node, name string, value any) {
	if n == nil {
		return
	}
	key := strings.ToLower(name)
	if r.Players() == 2 && n == r.Root && key == "busy" && r.playerContext > 0 {
		r.coopBusy[r.playerContext-1] = truth(value)
		value = float64(0)
		if r.coopBusy[0] || r.coopBusy[1] {
			value = float64(1)
		}
	}
	v := number(value)
	if r.Players() == 2 && n == r.Root && key == "score" {
		delta := v - number(n.Get("score"))
		if r.playerContext > 0 {
			r.coopScores[r.playerContext-1] += delta
		} else {
			// Shop actions have no miner context; both players share the cost.
			r.coopScores[0] += delta / 2
			r.coopScores[1] += delta / 2
		}
	}
	switch key {
	case "_x":
		n.Matrix.TX = v
	case "_y":
		n.Matrix.TY = v
	case "_visible":
		n.Visible = truth(value)
	case "_alpha":
		n.Color.Mul[3] = v / 100
	case "_xscale":
		angle := math.Atan2(n.Matrix.B, n.Matrix.A)
		n.Matrix.A = math.Cos(angle) * v / 100
		n.Matrix.B = math.Sin(angle) * v / 100
	case "_yscale":
		angle := math.Atan2(-n.Matrix.C, n.Matrix.D)
		n.Matrix.C = -math.Sin(angle) * v / 100
		n.Matrix.D = math.Cos(angle) * v / 100
	case "_rotation":
		sx, sy := math.Hypot(n.Matrix.A, n.Matrix.B), math.Hypot(n.Matrix.C, n.Matrix.D)
		a := v * math.Pi / 180
		n.Matrix.A = math.Cos(a) * sx
		n.Matrix.B = math.Sin(a) * sx
		n.Matrix.C = -math.Sin(a) * sy
		n.Matrix.D = math.Cos(a) * sy
	default:
		n.Vars[key] = value
	}
}
