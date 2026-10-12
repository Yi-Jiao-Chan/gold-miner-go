package flash

import (
	"bytes"
	"testing"
)

func cooperativeRuntime(t *testing.T) *Runtime {
	t.Helper()
	r := archivedRuntime(t)
	startGame(t, r)
	ticks(t, r, 40)
	if err := r.SetPlayers(2); err != nil {
		t.Fatal(err)
	}
	return r
}

func coopNode(t *testing.T, r *Runtime, path string) *Node {
	t.Helper()
	n, _ := r.Get(r.Root, path).(*Node)
	if n == nil {
		t.Fatalf("missing %s", path)
	}
	return n
}

func TestCooperativeAllLayoutsAndIndependentInput(t *testing.T) {
	movie := archivedMovie(t)
	count := 0
	for label := range movie.Characters[0].Labels {
		if len(label) < 4 || label[0] != 'L' || label[len(label)-2] != '_' {
			continue
		}
		count++
		r, err := New(movie, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.SetPlayers(2); err != nil {
			t.Fatal(err)
		}
		startGame(t, r)
		ticks(t, r, 40)
		if err := r.Goto(r.Root, label, false); err != nil {
			t.Fatal(err)
		}
		c1, c2 := coopNode(t, r, "C"), coopNode(t, r, "C2")
		m1, m2 := coopNode(t, r, "miner"), coopNode(t, r, "miner2")
		b1, b2 := coopNode(t, r, "bonus"), coopNode(t, r, "bonus2")
		if c1 == c2 || m1 == m2 || b1 == b2 || c1.Matrix.TX != 235 || c2.Matrix.TX != 305 || m2.Matrix.TX-m1.Matrix.TX != 70 {
			t.Fatalf("%s: player instances or anchors wrong", label)
		}
		timer := findCharacter(r.Root, 197)
		if number(timer.Get("time")) != 40 || number(r.Root.Get("time")) != 40 {
			t.Fatalf("%s: timer %v root time %v", label, timer.Get("time"), r.Root.Get("time"))
		}
		cl1, cl2 := coopNode(t, r, "C.cl"), coopNode(t, r, "C2.cl")
		if err := r.PlayerKey(2, 15); err != nil {
			t.Fatal(err)
		}
		if c2.Playing || !cl2.Playing || !c1.Playing || cl1.Playing || m2.Frame != 3 || m1.Frame != 1 {
			t.Fatalf("%s: player-two input changed player one", label)
		}
		if err := r.PlayerKey(1, 15); err != nil {
			t.Fatal(err)
		}
		ticks(t, r, 1)
		if cl1.Frame != 2 || cl2.Frame != 2 {
			t.Fatalf("%s: simultaneous drop frames %d/%d", label, cl1.Frame, cl2.Frame)
		}
		r.Set(timer, "time", float64(10000))
		r.Set(r.Root, "stick", float64(5))
		for tick := 0; tick < 600; tick++ {
			for player := 1; player <= 2; player++ {
				if tick%(79+player*11) == 0 {
					if err := r.PlayerKey(player, 15); err != nil {
						t.Fatalf("%s player %d drop: %v", label, player, err)
					}
				}
				if tick%(47+player*7) == 0 {
					if err := r.PlayerKey(player, 14); err != nil {
						t.Fatalf("%s player %d throw: %v", label, player, err)
					}
				}
			}
			if err := r.Tick(); err != nil {
				t.Fatalf("%s tick %d: %v", label, tick, err)
			}
		}
	}
	if count != 30 {
		t.Fatalf("layouts=%d", count)
	}
}

func isolateCoopTargets(t *testing.T, r *Runtime) {
	t.Helper()
	for i := 1; i <= int(number(r.Root.Get("things"))); i++ {
		coopNode(t, r, "T"+String(float64(i))).Matrix.TX += 10000
	}
}

func alignCoopTarget(t *testing.T, r *Runtime, target, sensor *Node) {
	t.Helper()
	box, _ := r.Get(target, "ob.box").(*Node)
	if box == nil {
		t.Fatal("target has no collision box")
	}
	a, b := r.Bounds(sensor), r.Bounds(box)
	target.Matrix.TX += a.X - b.X
	target.Matrix.TY += a.Y - b.Y
}

func TestCooperativeSameTargetCollectedOnlyOnce(t *testing.T) {
	r := cooperativeRuntime(t)
	if err := r.Goto(r.Root, "L1_1", false); err != nil {
		t.Fatal(err)
	}
	isolateCoopTargets(t, r)
	for _, path := range []string{"C", "C2"} {
		c := coopNode(t, r, path)
		if err := r.Goto(c, 3, false); err != nil {
			t.Fatal(err)
		}
		if err := r.Goto(coopNode(t, r, path+".cl"), 2, false); err != nil {
			t.Fatal(err)
		}
	}
	// Force both original collision sensors onto one target in the same tick.
	coopNode(t, r, "C2").Matrix = coopNode(t, r, "C").Matrix
	target := coopNode(t, r, "T1")
	price := number(r.Get(target, "ob.price"))
	alignCoopTarget(t, r, target, coopNode(t, r, "C.cl.box"))
	ticks(t, r, 1)
	cl1, cl2 := coopNode(t, r, "C.cl"), coopNode(t, r, "C2.cl")
	if number(cl1.Get("busy"))+number(cl2.Get("busy")) != 1 || target.Frame != 2 {
		t.Fatalf("duplicate claim: busy=%v/%v target=%d", cl1.Get("busy"), cl2.Get("busy"), target.Frame)
	}
	ticks(t, r, 450)
	if number(r.Root.Get("score")) != price {
		t.Fatalf("shared target credited %v, want %v", r.Root.Get("score"), price)
	}
}

func TestCooperativeDistinctTargetsShareScore(t *testing.T) {
	r := cooperativeRuntime(t)
	if err := r.Goto(r.Root, "L1_1", false); err != nil {
		t.Fatal(err)
	}
	isolateCoopTargets(t, r)
	price := float64(0)
	for i, path := range []string{"C", "C2"} {
		if err := r.Goto(coopNode(t, r, path), 3, false); err != nil {
			t.Fatal(err)
		}
		if err := r.Goto(coopNode(t, r, path+".cl"), 2, false); err != nil {
			t.Fatal(err)
		}
		target := coopNode(t, r, "T"+String(float64(i+1)))
		price += number(r.Get(target, "ob.price"))
		alignCoopTarget(t, r, target, coopNode(t, r, path+".cl.box"))
	}
	ticks(t, r, 1)
	if number(coopNode(t, r, "C.cl").Get("busy")) != 1 || number(coopNode(t, r, "C2.cl").Get("busy")) != 1 {
		t.Fatal("both original claws did not collect independent targets")
	}
	ticks(t, r, 450)
	if number(r.Root.Get("score")) != price {
		t.Fatalf("shared score=%v want %v", r.Root.Get("score"), price)
	}
}

func TestCooperativeParallelBonusesAndSharedInventory(t *testing.T) {
	r := cooperativeRuntime(t)
	b1, b2 := coopNode(t, r, "bonus"), coopNode(t, r, "bonus2")
	c1, c2 := coopNode(t, r, "C"), coopNode(t, r, "C2")
	c1.Playing, c2.Playing = false, false
	r.Set(b1, "bscore", float64(41))
	if err := r.Goto(b1, 2, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 10)
	r.Set(b2, "bscore", float64(73))
	if err := r.Goto(b2, 2, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 22)
	if number(r.Root.Get("score")) != 41 || number(r.Root.Get("busy")) != 1 || !c1.Playing || c2.Playing {
		t.Fatalf("first bonus interfered: score=%v busy=%v swings=%v/%v", r.Root.Get("score"), r.Root.Get("busy"), c1.Playing, c2.Playing)
	}
	// Player one's completed bonus must not let the clock end the level while
	// player two's original credit animation is still running.
	timer := findCharacter(r.Root, 197)
	r.Set(timer, "time", float64(1))
	layout := r.Root.Frame
	if err := r.Goto(timer, 19, true); err != nil {
		t.Fatal(err)
	}
	if r.Root.Frame != layout || number(timer.Get("time")) != 0 {
		t.Fatal("timer ignored player-two unfinished bonus")
	}
	ticks(t, r, 12)
	if number(r.Root.Get("score")) != 114 || number(r.Root.Get("busy")) != 0 || !c2.Playing {
		t.Fatalf("parallel credit: score=%v busy=%v", r.Root.Get("score"), r.Root.Get("busy"))
	}
	r.Set(timer, "time", float64(10000))
	for _, reward := range []string{"power", "dynamite"} {
		c1.Playing, c2.Playing = false, false
		r.Set(b2, "bscore", reward)
		if err := r.Goto(b2, 2, true); err != nil {
			t.Fatal(err)
		}
		ticks(t, r, 90)
		if c1.Playing || !c2.Playing || number(r.Root.Get("score")) != 114 {
			t.Fatalf("%s reward resumed wrong miner or altered score", reward)
		}
	}
	if number(r.Root.Get("strength")) != 1 || number(r.Root.Get("stick")) != 1 {
		t.Fatalf("inventory strength=%v stick=%v", r.Root.Get("strength"), r.Root.Get("stick"))
	}
}

func TestCooperativePlayerTwoDynamiteTimeline(t *testing.T) {
	r := cooperativeRuntime(t)
	if err := r.Goto(r.Root, "L1_1", false); err != nil {
		t.Fatal(err)
	}
	isolateCoopTargets(t, r)
	r.Set(r.Root, "stick", float64(1))
	c2 := coopNode(t, r, "C2")
	if err := r.Goto(c2, 3, false); err != nil {
		t.Fatal(err)
	}
	cl2 := coopNode(t, r, "C2.cl")
	if err := r.Goto(cl2, 41, false); err != nil {
		t.Fatal(err)
	}
	held := coopNode(t, r, "T9")
	alignCoopTarget(t, r, held, coopNode(t, r, "C2.cl.box"))
	ticks(t, r, 1)
	if number(cl2.Get("busy")) != 1 || number(cl2.Get("weight")) != 9 {
		t.Fatal("player two failed to catch rock")
	}
	if err := r.PlayerKey(2, 14); err != nil {
		t.Fatal(err)
	}
	if number(r.Root.Get("stick")) != 0 || coopNode(t, r, "miner").Frame != 1 {
		t.Fatal("player-two dynamite changed the wrong inventory/miner")
	}
	ticks(t, r, 100)
	if number(cl2.Get("price")) != 0 || number(cl2.Get("stickBonus")) != 1 || number(coopNode(t, r, "C.cl").Get("stickBonus")) != 0 {
		t.Fatalf("player-two throw targeted wrong claw: price=%v boost=%v", cl2.Get("price"), cl2.Get("stickBonus"))
	}
}

func TestCooperativeTimerNextLevelAndSoloRestoration(t *testing.T) {
	r := cooperativeRuntime(t)
	timer := findCharacter(r.Root, 197)
	ticks(t, r, 18)
	if number(timer.Get("time")) != 39 {
		t.Fatalf("after second: %v", timer.Get("time"))
	}
	r.Set(r.Root, "score", float64(10000))
	r.Set(timer, "time", float64(1))
	ticks(t, r, 220)
	if r.Root.Frame != 130 || r.Players() != 2 || r.Get(r.Root, "C2") != nil {
		t.Fatalf("shop frame=%d players=%d", r.Root.Frame, r.Players())
	}
	if err := r.Goto(r.Root, "next", true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 50)
	coopNode(t, r, "C2")
	if number(findCharacter(r.Root, 197).Get("time")) != 40 {
		t.Fatal("new level did not reset cooperative timer")
	}
	if err := r.SetPlayers(1); err != nil {
		t.Fatal(err)
	}
	if r.Get(r.Root, "C2") != nil || coopNode(t, r, "C").Matrix.TX != 275.5 || number(findCharacter(r.Root, 197).Get("time")) != 60 {
		t.Fatal("solo restoration failed")
	}
	if r.SetPlayers(3) == nil || r.PlayerKey(2, 15) == nil {
		t.Fatal("invalid player configuration accepted")
	}
}

func TestCooperativeFortySecondsAcrossFreshLevels(t *testing.T) {
	// The first level enables co-op after the original layout was already live;
	// later levels exercise the automatic recreation of the original timer.
	r := cooperativeRuntime(t)
	for level := 1; level <= 4; level++ {
		if level > 1 {
			if err := r.Goto(r.Root, "next", true); err != nil {
				t.Fatal(err)
			}
			for i := 0; findCharacter(r.Root, 197) == nil && i < 100; i++ {
				ticks(t, r, 1)
			}
		}
		timer := findCharacter(r.Root, 197)
		if timer == nil || timer.Frame != 1 || number(timer.Get("time")) != 40 {
			t.Fatalf("level %d did not start a fresh forty-second timer", level)
		}
		r.Set(r.Root, "score", float64(100000))
		for second := 1; second <= 39; second++ {
			ticks(t, r, 18)
			if r.Get(r.Root, "C") == nil || number(timer.Get("time")) != float64(40-second) {
				t.Fatalf("level %d second %d: time=%v frame=%d", level, second, timer.Get("time"), r.Root.Frame)
			}
		}
		ticks(t, r, 17)
		if r.Get(r.Root, "C") == nil || number(timer.Get("time")) != 1 {
			t.Fatalf("level %d ended before tick 720", level)
		}
		ticks(t, r, 1)
		if r.Get(r.Root, "C") != nil || number(timer.Get("time")) != 0 {
			t.Fatalf("level %d did not end on tick 720", level)
		}
	}
}

func TestCooperativeTimerCheckpointPreservesPendingHold(t *testing.T) {
	r := cooperativeRuntime(t)
	ticks(t, r, 18)
	timer := findCharacter(r.Root, 197)
	if timer.Frame != 2 || !truth(timer.Get("_cooptimerhold")) {
		t.Fatal("first decrement did not schedule a timer hold")
	}
	data, err := r.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(r.Movie, data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		ticks(t, r, 1)
		ticks(t, restored, 1)
		a, err := r.Checkpoint()
		if err != nil {
			t.Fatal(err)
		}
		b, err := restored.Checkpoint()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("timer checkpoint diverged after %d ticks", i+1)
		}
		if i == 0 && timer.Frame != 2 {
			t.Fatal("restored timer lost its pending one-tick hold")
		}
	}
}

func TestCooperativeFortySecondExpiryWaitsForBothBonuses(t *testing.T) {
	r := cooperativeRuntime(t)
	r.Set(r.Root, "score", float64(10000))
	ticks(t, r, 39*18)
	b1, b2 := coopNode(t, r, "bonus"), coopNode(t, r, "bonus2")
	r.Set(b1, "bscore", float64(11))
	if err := r.Goto(b1, 2, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 10)
	r.Set(b2, "bscore", float64(73))
	if err := r.Goto(b2, 2, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 8)
	timer := findCharacter(r.Root, 197)
	if timer == nil || number(timer.Get("time")) != 0 || number(r.Root.Get("busy")) != 1 {
		t.Fatal("expiry removed pending cooperative bonus timelines")
	}
	ticks(t, r, 14)
	if number(r.Root.Get("score")) != 10011 || number(r.Root.Get("busy")) != 1 {
		t.Fatal("first completed bonus stopped waiting for player two")
	}
	ticks(t, r, 10)
	if number(r.Root.Get("score")) != 10084 || number(r.Root.Get("busy")) != 0 {
		t.Fatal("second pending bonus did not finish its shared score credit")
	}
	ticks(t, r, 12)
	if r.Get(r.Root, "C") != nil {
		t.Fatal("expired level did not finish after both bonuses completed")
	}
}

func TestSoloTimerRetainsArchivedPacing(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	for findCharacter(r.Root, 197) == nil {
		ticks(t, r, 1)
	}
	timer := findCharacter(r.Root, 197)
	ticks(t, r, 18)
	if number(timer.Get("time")) != 59 {
		t.Fatal("solo first-second timing changed")
	}
	ticks(t, r, 17)
	if number(timer.Get("time")) != 58 {
		t.Fatal("solo original seventeen-frame timing changed")
	}
	r.Set(timer, "time", float64(11))
	if err := r.Goto(timer, 19, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 18)
	if number(timer.Get("time")) != 10 {
		t.Fatal("solo warning timing changed")
	}
	ticks(t, r, 1)
	if number(timer.Get("time")) != 9 {
		t.Fatal("solo original nineteen-frame warning timing changed")
	}
}
