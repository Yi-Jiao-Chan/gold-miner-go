package flash

import (
	"fmt"
	"strings"
)

const coopLeftAnchor = 235.0
const coopRightAnchor = 305.0

// Players returns the configured number of miners, including between levels.
func (r *Runtime) Players() int {
	if r.players == 2 {
		return 2
	}
	return 1
}

// SetPlayers selects solo or cooperative play. Cooperative miners use separate
// instances of the original timelines, with one shared field and inventory.
func (r *Runtime) SetPlayers(players int) error {
	if players != 1 && players != 2 {
		return fmt.Errorf("players must be 1 or 2")
	}
	if r.Players() == players {
		return nil
	}
	busy := truth(r.Root.Vars["busy"])
	if players == 1 {
		busy = r.coopBusy[0]
	}
	r.players = players
	r.coopBusy = [2]bool{busy, false}
	r.Root.Vars["busy"] = float64(0)
	if busy {
		r.Root.Vars["busy"] = float64(1)
	}
	if err := r.display(r.Root, r.Root.Def.Frames[r.Root.Frame-1].Display); err != nil {
		return r.fail(err)
	}
	// Enabling in an already initialized layout does not rerun the timer's
	// frame-one script, so update that live instance explicitly.
	for _, n := range r.Root.Children {
		if n.Def.ID == 197 {
			seconds := float64(60)
			if players == 2 {
				seconds = 40
			}
			r.Root.Vars["time"] = seconds
			n.Vars["time"] = seconds
			delete(n.Vars, "_cooptimerhold")
			if players == 2 {
				if err := r.seek(n, 1, true); err != nil {
					return r.fail(err)
				}
			}
		}
	}
	return r.drain()
}

// PlayerKey sends an original SWF key code to one miner (1 or 2). Key remains
// the single-player convenience entry point and controls player one in co-op.
func (r *Runtime) PlayerKey(player, code int) error {
	if player < 1 || player > r.Players() {
		return fmt.Errorf("player %d is unavailable", player)
	}
	return r.keyForPlayer(player, code)
}

func coopPlayer(name string) int {
	switch strings.ToLower(name) {
	case "c", "miner", "bonus":
		return 1
	case "c2", "miner2", "bonus2":
		return 2
	}
	return 0
}

func (r *Runtime) coopPlacements(ps []Placement) []Placement {
	if r.Players() != 2 {
		return ps
	}
	// These three named root instances coexist only on the original layouts.
	count, maxDepth := 0, 0
	for _, p := range ps {
		if coopPlayer(p.Name) == 1 {
			count++
		}
		if p.Depth > maxDepth {
			maxDepth = p.Depth
		}
	}
	if count != 3 {
		return ps
	}
	out := make([]Placement, 0, len(ps)+3)
	for _, p := range ps {
		if p.Character == 197 {
			old := r.Root.Children[p.Depth]
			if old == nil || old.Def.ID != p.Character || old.placement.Born != p.Born {
				r.Root.Vars["time"] = float64(40)
				r.coopBusy = [2]bool{}
			}
		}
		if coopPlayer(p.Name) != 1 {
			out = append(out, p)
			continue
		}
		// The authored claw anchor is x=275.5; translating all three instances
		// equally retains each miner's rope/throw/bonus geometry.
		p.Matrix.TX += coopLeftAnchor - 275.5
		out = append(out, p)
		p.Name += "2"
		maxDepth++
		p.Depth = maxDepth
		p.Matrix.TX += coopRightAnchor - coopLeftAnchor
		out = append(out, p)
	}
	return out
}

// Update the spacing of older cooperative saves without seeking timelines or
// resetting a moving claw, held item, bonus, inventory or random state.
func (r *Runtime) restoreCoopSpacing() {
	if r.Players() != 2 {
		return
	}
	for _, p := range r.Root.Def.Frames[r.Root.Frame-1].Display {
		if coopPlayer(p.Name) != 1 {
			continue
		}
		for _, n := range r.Root.Children {
			player := coopPlayer(n.Name)
			if player == 0 || n.Def.ID != p.Character || n.placement.Born != p.Born || (n.Name != p.Name && n.Name != p.Name+"2") {
				continue
			}
			x := p.Matrix.TX + (coopLeftAnchor - 275.5)
			if player == 2 {
				x += coopRightAnchor - coopLeftAnchor
			}
			n.Matrix.TX += x - n.placement.Matrix.TX
			n.placement.Matrix.TX = x
		}
	}
}

// The authored clock decrements first after 18 frames, then every 17 frames
// above ten seconds and every 19 frames during its warning animation. Co-op
// promises 40 real seconds, so make both subsequent paths exactly 18 frames.
func (r *Runtime) coopTimerAction(n *Node, frame int) {
	if r.Players() != 2 || n.Def.ID != 197 || frame != 19 {
		return
	}
	delete(n.Vars, "_cooptimerhold")
	if n.Frame == 2 {
		// Keep the pending hold in Vars so an in-flight checkpoint preserves it.
		n.Vars["_cooptimerhold"] = true
	}
}

func (r *Runtime) coopTimerStep(n *Node, next int) (int, bool) {
	if r.Players() != 2 || n.Def.ID != 197 {
		return next, true
	}
	if n.Frame == 2 && truth(n.Get("_cooptimerhold")) {
		delete(n.Vars, "_cooptimerhold")
		return next, false
	}
	if n.Frame == 20 {
		// Frame 21 has no actions; retain the warning and its frame-22 loop.
		next = 22
	}
	return next, true
}
