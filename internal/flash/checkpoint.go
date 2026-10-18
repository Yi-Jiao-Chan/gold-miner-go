package flash

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
)

// Checkpoint serializes a quiescent movie without replaying initialization or
// consuming random numbers. Variables referring to clips retain their identity.
func (r *Runtime) Checkpoint() ([]byte, error) {
	if r.LastError != nil {
		return nil, r.LastError
	}
	if r.draining || len(r.jobs) != 0 {
		return nil, fmt.Errorf("movie is executing actions")
	}
	cp := checkpoint{Version: 1, Movie: r.Movie.SHA256, Players: r.Players(), Busy: r.coopBusy}
	if r.Players() == 2 {
		scores := r.PlayerScores()
		cp.PlayerScores = &scores
	}
	var err error
	cp.Random, err = r.pcg.MarshalBinary()
	if err != nil {
		return nil, err
	}
	nodes := []*Node{}
	ids := map[*Node]int{}
	var visit func(*Node)
	visit = func(n *Node) {
		if n == nil || ids[n] != 0 {
			return
		}
		ids[n] = len(nodes) + 1
		nodes = append(nodes, n)
		for _, c := range ordered(n) {
			visit(c)
		}
	}
	visit(r.Root)
	// Keep any clip-valued variable, even if that clip was just removed.
	for i := 0; i < len(nodes); i++ {
		for _, v := range nodes[i].Vars {
			if n, ok := v.(*Node); ok {
				visit(n)
			}
		}
	}
	encode := func(v any) (savedValue, error) {
		switch x := v.(type) {
		case nil:
			return savedValue{Kind: "nil"}, nil
		case string:
			return savedValue{Kind: "string", String: x}, nil
		case bool:
			return savedValue{Kind: "bool", Bool: x}, nil
		case *Node:
			return savedValue{Kind: "node", Ref: ids[x]}, nil
		case float64, float32, int, uint32:
			return savedValue{Kind: "number", String: strconv.FormatFloat(number(v), 'g', -1, 64)}, nil
		default:
			return savedValue{}, fmt.Errorf("unsupported checkpoint variable %T", v)
		}
	}
	for _, n := range nodes {
		s := savedNode{Character: n.Def.ID, Parent: ids[n.Parent], Name: n.Name, Matrix: n.Matrix, Color: n.Color, Ratio: n.Ratio, ClipDepth: n.ClipDepth, Visible: n.Visible, Frame: n.Frame, Playing: n.Playing, ButtonState: n.ButtonState, Placement: n.placement, Revision: n.revision, Initialized: n.initialized, Player: n.player, Children: map[int]int{}, Vars: map[string]savedValue{}}
		for d, c := range n.Children {
			s.Children[d] = ids[c]
		}
		for k, v := range n.Vars {
			value, e := encode(v)
			if e != nil {
				return nil, e
			}
			s.Vars[k] = value
		}
		cp.Nodes = append(cp.Nodes, s)
	}
	return json.Marshal(cp)
}

// Restore creates a movie directly from a saved display tree. It deliberately
// does not seek frames: shop prices, purchases and grab-bag contents stay fixed.
func Restore(movie *Movie, data []byte) (*Runtime, error) {
	if len(data) > 8<<20 {
		return nil, fmt.Errorf("checkpoint too large")
	}
	var cp checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	if cp.Version != 1 || movie == nil || cp.Movie != movie.SHA256 || (cp.Players != 1 && cp.Players != 2) || len(cp.Nodes) == 0 || len(cp.Nodes) > 10000 {
		return nil, fmt.Errorf("unsupported checkpoint")
	}
	pcg := rand.NewPCG(0, 0)
	if err := pcg.UnmarshalBinary(cp.Random); err != nil {
		return nil, err
	}
	r := &Runtime{Movie: movie, pcg: pcg, rng: rand.New(pcg), players: cp.Players, coopBusy: cp.Busy}
	nodes := make([]*Node, len(cp.Nodes))
	for i, s := range cp.Nodes {
		d := movie.Characters[s.Character]
		if d == nil || s.Frame < 1 || (len(d.Frames) > 0 && s.Frame > len(d.Frames)) || s.Player < 0 || s.Player > 2 {
			return nil, fmt.Errorf("invalid saved clip %d", i)
		}
		nodes[i] = &Node{Def: d, Name: s.Name, Matrix: s.Matrix, Color: s.Color, Ratio: s.Ratio, ClipDepth: s.ClipDepth, Visible: s.Visible, Frame: s.Frame, Playing: s.Playing, ButtonState: s.ButtonState, placement: s.Placement, revision: s.Revision, initialized: s.Initialized, player: s.Player, Children: map[int]*Node{}, Vars: map[string]any{}}
	}
	ref := func(id int) (*Node, error) {
		if id == 0 {
			return nil, nil
		}
		if id < 1 || id > len(nodes) {
			return nil, fmt.Errorf("invalid saved reference")
		}
		return nodes[id-1], nil
	}
	for i, s := range cp.Nodes {
		n := nodes[i]
		var err error
		n.Parent, err = ref(s.Parent)
		if err != nil {
			return nil, err
		}
		for depth, id := range s.Children {
			c, e := ref(id)
			if e != nil || c == nil || c == n || cp.Nodes[id-1].Parent != i+1 {
				return nil, fmt.Errorf("invalid saved display tree")
			}
			n.Children[depth] = c
		}
		for k, v := range s.Vars {
			switch v.Kind {
			case "nil":
				n.Vars[k] = nil
			case "string":
				n.Vars[k] = v.String
			case "bool":
				n.Vars[k] = v.Bool
			case "number":
				x, e := strconv.ParseFloat(v.String, 64)
				if e != nil {
					return nil, e
				}
				n.Vars[k] = x
			case "node":
				x, e := ref(v.Ref)
				if e != nil {
					return nil, e
				}
				n.Vars[k] = x
			default:
				return nil, fmt.Errorf("invalid saved value")
			}
		}
	}
	r.Root = nodes[0]
	if r.Root.Parent != nil || r.Root.Def.ID != 0 {
		return nil, fmt.Errorf("invalid saved root")
	}
	// Reject cycles before any renderer or timeline walker can traverse them.
	for _, n := range nodes {
		seen := map[*Node]bool{}
		for p := n; p != nil; p = p.Parent {
			if seen[p] {
				return nil, fmt.Errorf("cyclic saved tree")
			}
			seen[p] = true
		}
	}
	if r.Players() == 2 {
		if cp.PlayerScores == nil {
			// Legacy saves only have a shared score. Split it once, then retain
			// the individual balances in all subsequent checkpoints.
			half := number(r.Root.Get("score")) / 2
			r.coopScores = [2]float64{half, half}
		} else {
			r.coopScores = *cp.PlayerScores
			if r.coopScores[0]+r.coopScores[1] != number(r.Root.Get("score")) {
				return nil, fmt.Errorf("inconsistent saved player scores")
			}
		}
	}
	r.restoreCoopSpacing()
	return r, nil
}

type checkpoint struct {
	Version      int         `json:"version"`
	Movie        string      `json:"movie"`
	Players      int         `json:"players"`
	PlayerScores *[2]float64 `json:"player_scores,omitempty"`
	Busy         [2]bool     `json:"busy"`
	Random       []byte      `json:"random"`
	Nodes        []savedNode `json:"nodes"`
}
type savedValue struct {
	Kind   string `json:"kind"`
	String string `json:"string,omitempty"`
	Bool   bool   `json:"bool,omitempty"`
	Ref    int    `json:"ref,omitempty"`
}
type savedNode struct {
	Character, Parent int
	Name              string
	Matrix            Matrix
	Color             ColorTransform
	Ratio, ClipDepth  int
	Visible           bool
	Frame             int
	Playing           bool
	ButtonState       int
	Placement         Placement
	Revision          uint64
	Initialized       bool
	Player            int
	Children          map[int]int
	Vars              map[string]savedValue
}
