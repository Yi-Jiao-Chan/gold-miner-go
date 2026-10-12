package flash

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func archivedMovie(t *testing.T) *Movie {
	t.Helper()
	data, err := os.ReadFile("../../assets/movie.json")
	if err != nil {
		t.Fatal(err)
	}
	var movie Movie
	if err = json.Unmarshal(data, &movie); err != nil {
		t.Fatal(err)
	}
	return &movie
}
func archivedRuntime(t *testing.T) *Runtime {
	t.Helper()
	r, err := New(archivedMovie(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func ticks(t *testing.T, r *Runtime, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if err := r.Tick(); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}
}
func findCharacter(n *Node, id int) *Node {
	if n.Def.ID == id {
		return n
	}
	for _, c := range ordered(n) {
		if got := findCharacter(c, id); got != nil {
			return got
		}
	}
	return nil
}
func startGame(t *testing.T, r *Runtime) {
	t.Helper()
	ticks(t, r, 9)
	if r.Root.Frame != 10 || r.Root.Playing {
		t.Fatalf("intro frame=%d playing=%v", r.Root.Frame, r.Root.Playing)
	}
	button := findCharacter(r.Root, 35)
	if button == nil {
		t.Fatal("start button missing")
	}
	r.buttonEvent(button, "release")
	if err := r.drain(); err != nil {
		t.Fatal(err)
	}
}
func TestArchivedStartupAndLayouts(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	if r.Get(r.Root, "goal") != float64(650) || r.Get(r.Root, "level") != float64(1) {
		t.Fatalf("startup goal=%v level=%v", r.Get(r.Root, "goal"), r.Get(r.Root, "level"))
	}
	if r.Get(r.Root, "goalAddOn") != r.Get(r.Root, "goalAddon") {
		t.Fatal("Flash5 names must be case insensitive")
	}
	ticks(t, r, 40)
	if r.Root.Playing {
		t.Fatalf("level layout is not stopped at %d", r.Root.Frame)
	}
	labels := 0
	for label, frame := range r.Root.Def.Labels {
		if len(label) >= 4 && label[0] == 'L' && label[len(label)-2] == '_' {
			labels++
			if err := r.Goto(r.Root, label, false); err != nil {
				t.Fatalf("%s: %v", label, err)
			}
			if r.Root.Frame != frame {
				t.Fatalf("layout %s frame=%d expected %d", label, r.Root.Frame, frame)
			}
			things := int(number(r.Get(r.Root, "things")))
			if things < 1 {
				t.Fatalf("layout %s no things", label)
			}
			for i := 1; i <= things; i++ {
				p := "_root.T" + stringValue(float64(i)) + ".ob"
				ob, _ := r.Get(r.Root, p).(*Node)
				if ob == nil {
					t.Fatalf("layout %s missing %s", label, p)
				}
				if r.Get(ob, "object") == nil || r.Get(ob, "weight") == nil || r.Get(ob, "price") == nil {
					t.Fatalf("layout %s object uninitialized %s: %#v", label, p, ob.Vars)
				}
			}
		}
	}
	if labels != 30 {
		t.Fatalf("layouts=%d", labels)
	}
}
func TestArchivedTimerAndShop(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	ticks(t, r, 40)
	timer, _ := r.Get(r.Root, "clock").(*Node)
	if timer == nil {
		timer = findCharacter(r.Root, 197)
	}
	if timer == nil {
		t.Fatal("timer missing")
	}
	if number(r.Get(timer, "time")) != 60 {
		t.Fatalf("timer starts %v", r.Get(timer, "time"))
	}
	ticks(t, r, 18)
	if number(r.Get(timer, "time")) != 59 {
		t.Fatalf("after second: %v", r.Get(timer, "time"))
	}
	r.Set(r.Root, "score", float64(10000))
	r.Set(timer, "time", float64(1))
	ticks(t, r, 220)
	if r.Root.Frame != 130 || r.Root.Playing {
		t.Fatalf("shop frame=%d playing=%v", r.Root.Frame, r.Root.Playing)
	}
	buy := findCharacter(r.Root, 89)
	if buy == nil {
		t.Fatal("shop dynamite button missing")
	}
	price, before := number(r.Get(buy.Parent, "price")), number(r.Get(r.Root, "score"))
	r.buttonEvent(buy, "release")
	if err := r.drain(); err != nil {
		t.Fatal(err)
	}
	if number(r.Get(r.Root, "stick")) != 1 || number(r.Get(r.Root, "score")) != before-price {
		t.Fatalf("shop purchase sticks=%v score=%v price=%v", r.Get(r.Root, "stick"), r.Get(r.Root, "score"), price)
	}
}
func TestArchivedCollectionUsesOriginalBonusTimeline(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	ticks(t, r, 40)
	c, _ := r.Get(r.Root, "C").(*Node)
	if c == nil {
		t.Fatal("claw missing")
	}
	if err := r.Goto(c, 3, false); err != nil {
		t.Fatal(err)
	}
	c, _ = r.Get(c, "cl").(*Node)
	if c == nil {
		t.Fatal("inner claw missing")
	}
	if err := r.Goto(c, 2, false); err != nil {
		t.Fatal(err)
	}
	sensor := findCharacter(c, 266)
	box, _ := r.Get(r.Root, "T1.ob.box").(*Node)
	if sensor == nil || box == nil {
		t.Fatal("native collision geometry missing")
	}
	sw, bw := r.Bounds(sensor), r.Bounds(box)
	target, _ := r.Get(r.Root, "T1").(*Node)
	target.Matrix.TX += sw.X - bw.X
	target.Matrix.TY += sw.Y - bw.Y
	price := number(r.Get(r.Root, "T1.ob.price"))
	ticks(t, r, 1)
	if number(r.Get(c, "busy")) != 1 || target.Frame != 2 {
		t.Fatalf("collection busy=%v targetframe=%d", r.Get(c, "busy"), target.Frame)
	}
	bonus, _ := r.Get(r.Root, "bonus").(*Node)
	if bonus == nil {
		t.Fatal("bonus timeline missing")
	}
	for i := 0; bonus.Frame != 4 && i < 500; i++ {
		ticks(t, r, 1)
	}
	if bonus.Frame != 4 {
		t.Fatalf("bonus did not start, claw frame=%d", c.Frame)
	}
	if number(r.Get(r.Root, "score")) != 0 {
		t.Fatal("score credited before bonus")
	}
	ticks(t, r, 27)
	if bonus.Frame != 31 || number(r.Get(r.Root, "score")) != 0 {
		t.Fatalf("before credit bonus=%d score=%v", bonus.Frame, r.Get(r.Root, "score"))
	}
	ticks(t, r, 1)
	if bonus.Frame != 32 || number(r.Get(r.Root, "score")) != price {
		t.Fatalf("price=%v score=%v", price, r.Get(r.Root, "score"))
	}
}
func TestVMOperandsAndFailure(t *testing.T) {
	movie := &Movie{Characters: map[int]*Character{0: {ID: 0, Kind: "sprite", Frames: []Frame{{}}}}}
	r, err := New(movie, 1)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := hex.DecodeString("9611000076616c756500070a00000007030000000b1d00")
	if err = r.run(r.Root, code); err != nil {
		t.Fatal(err)
	}
	if r.Get(r.Root, "value") != float64(7) {
		t.Fatalf("operand order: %v", r.Get(r.Root, "value"))
	}
	if err = r.run(r.Root, []byte{0x55, 0}); err == nil {
		t.Fatal("unknown opcode accepted")
	}
	if err = r.run(r.Root, []byte{0x17, 0}); err == nil {
		t.Fatal("stack underflow accepted")
	}
}

func TestArchivedPointerAndKeyInput(t *testing.T) {
	r := archivedRuntime(t)
	ticks(t, r, 9)
	button := findCharacter(r.Root, 35)
	p := button.Def.States[8][0]
	def := r.Movie.Characters[p.Character]
	bounds := def.Asset.Bounds
	x, y := r.World(button).Mul(p.Matrix).Point(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
	for _, down := range []bool{false, true, false} {
		if err := r.Pointer(x, y, down); err != nil {
			t.Fatal(err)
		}
	}
	if r.Root.Frame != 15 || number(r.Get(r.Root, "goal")) != 650 {
		t.Fatalf("start click frame=%d goal=%v", r.Root.Frame, r.Get(r.Root, "goal"))
	}
	ticks(t, r, 40)
	c, _ := r.Get(r.Root, "C").(*Node)
	cl, _ := r.Get(c, "cl").(*Node)
	if cl == nil {
		t.Fatal("claw missing")
	}
	if err := r.Key(15); err != nil {
		t.Fatal(err)
	}
	if c.Playing || !cl.Playing {
		t.Fatalf("down key swing=%v claw=%v", c.Playing, cl.Playing)
	}
	ticks(t, r, 1)
	if cl.Frame != 2 {
		t.Fatalf("drop frame=%d", cl.Frame)
	}
}
func TestSeededPlaybackAndLevelGoal(t *testing.T) {
	a, b := archivedRuntime(t), archivedRuntime(t)
	startGame(t, a)
	startGame(t, b)
	ticks(t, a, 40)
	ticks(t, b, 40)
	if a.Root.Frame != b.Root.Frame {
		t.Fatalf("same seed layouts %d %d", a.Root.Frame, b.Root.Frame)
	}
	if err := a.Goto(a.Root, 52, true); err != nil {
		t.Fatal(err)
	}
	if number(a.Get(a.Root, "goal")) != 1195 || number(a.Get(a.Root, "levelDis")) != 2 {
		t.Fatalf("second level goal=%v level=%v", a.Get(a.Root, "goal"), a.Get(a.Root, "levelDis"))
	}
}

func TestArchivedLevelWrapAndGoalGrowth(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	ticks(t, r, 40)
	goal, addon := 650.0, 275.0
	for displayed := 2; displayed <= 14; displayed++ {
		if displayed <= 10 {
			addon += 270
		}
		goal += addon
		if err := r.Goto(r.Root, 52, true); err != nil {
			t.Fatal(err)
		}
		group := displayed
		if group > 10 {
			group = displayed - 7
		}
		if number(r.Get(r.Root, "levelDis")) != float64(displayed) || number(r.Get(r.Root, "level")) != float64(group) || number(r.Get(r.Root, "goal")) != goal || number(r.Get(r.Root, "goalAddon")) != addon {
			t.Fatalf("display %d group=%v goal=%v addon=%v", displayed, r.Get(r.Root, "level"), r.Get(r.Root, "goal"), r.Get(r.Root, "goalAddon"))
		}
		ticks(t, r, 40)
		if r.Root.Playing {
			t.Fatalf("display %d did not reach layout", displayed)
		}
		prefix := "L" + String(float64(group)) + "_"
		matched := false
		for label, frame := range r.Root.Def.Labels {
			if len(label) > len(prefix) && label[:len(prefix)] == prefix && frame == r.Root.Frame {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("display %d did not route to group %d: frame %d", displayed, group, r.Root.Frame)
		}
	}
	if addon != 2705 {
		t.Fatalf("frozen addon=%v", addon)
	}
}

func TestArchivedLayoutsSustainedInput(t *testing.T) {
	movie := archivedMovie(t)
	for group := 1; group <= 10; group++ {
		for variant := 1; variant <= 3; variant++ {
			label := "L" + String(float64(group)) + "_" + String(float64(variant))
			t.Run(label, func(t *testing.T) {
				r, err := New(movie, uint64(group*3+variant))
				if err != nil {
					t.Fatal(err)
				}
				startGame(t, r)
				ticks(t, r, 40)
				r.Set(r.Root, "stick", float64(5))
				r.Set(r.Root, "level", float64(group))
				if err = r.Goto(r.Root, label, false); err != nil {
					t.Fatal(err)
				}
				timer := findCharacter(r.Root, 197)
				if timer == nil {
					t.Fatal("timer missing")
				}
				r.Set(timer, "time", float64(10000))
				for i := 0; i < 2500; i++ {
					if i%101 == 0 {
						if err = r.Key(15); err != nil {
							t.Fatalf("down at %d: %v", i, err)
						}
					}
					if i%53 == 0 {
						if err = r.Key(14); err != nil {
							t.Fatalf("up at %d: %v", i, err)
						}
					}
					if err = r.Tick(); err != nil {
						t.Fatalf("tick %d: %v", i, err)
					}
				}
			})
		}
	}
}

func TestArchivedDynamiteDestroysHeldAndNearbyObjects(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	ticks(t, r, 40)
	if err := r.Goto(r.Root, "L1_1", false); err != nil {
		t.Fatal(err)
	}
	r.Set(r.Root, "stick", float64(1))
	swing, _ := r.Get(r.Root, "C").(*Node)
	if err := r.Goto(swing, 3, false); err != nil {
		t.Fatal(err)
	}
	cl, _ := r.Get(swing, "cl").(*Node)
	if err := r.Goto(cl, 41, false); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= int(number(r.Get(r.Root, "things"))); i++ {
		target, _ := r.Get(r.Root, "T"+String(float64(i))).(*Node)
		target.Matrix.TX += 10000
	}
	held, _ := r.Get(r.Root, "T9").(*Node)
	box, _ := r.Get(held, "ob.box").(*Node)
	sensor, _ := r.Get(cl, "box").(*Node)
	sw, bw := r.Bounds(sensor), r.Bounds(box)
	held.Matrix.TX += sw.X - bw.X
	held.Matrix.TY += sw.Y - bw.Y
	ticks(t, r, 1)
	if number(r.Get(cl, "busy")) != 1 || held.Frame != 2 || number(r.Get(cl, "weight")) != 9 {
		t.Fatalf("rock not held: busy=%v frame=%d weight=%v", r.Get(cl, "busy"), held.Frame, r.Get(cl, "weight"))
	}
	if err := r.Key(14); err != nil {
		t.Fatal(err)
	}
	if number(r.Get(r.Root, "stick")) != 0 {
		t.Fatalf("Up consumed %v sticks", r.Get(r.Root, "stick"))
	}
	nearby, _ := r.Get(r.Root, "T1").(*Node)
	for i := 0; i < 200; i++ {
		// Place the adjacent original target inside the original explosion bounds.
		if fx := findCharacter(cl, 263); fx != nil {
			ob, _ := r.Get(nearby, "ob.box").(*Node)
			a, b := r.Bounds(fx), r.Bounds(ob)
			if a.W > 0 && a.H > 0 {
				nearby.Matrix.TX += a.X + a.W/2 - b.X - b.W/2
				nearby.Matrix.TY += a.Y + a.H/2 - b.Y - b.H/2
			}
		}
		ticks(t, r, 1)
		if number(r.Get(cl, "price")) == 0 && nearby.Frame == 2 {
			break
		}
	}
	claw, _ := r.Get(cl, "claw").(*Node)
	if number(r.Get(cl, "price")) != 0 || number(r.Get(cl, "stickBonus")) != 1 || nearby.Frame != 2 || claw == nil || claw.Frame != 20 {
		t.Fatalf("explosion price=%v boost=%v nearby=%d claw=%v", r.Get(cl, "price"), r.Get(cl, "stickBonus"), nearby.Frame, claw)
	}
	if number(r.Get(r.Root, "score")) != 0 {
		t.Fatalf("destroyed rock credited score=%v", r.Get(r.Root, "score"))
	}
}

func objectRuntime(t *testing.T, movie *Movie, id int, seed uint64) *Runtime {
	t.Helper()
	chars := make(map[int]*Character, len(movie.Characters))
	for k, v := range movie.Characters {
		chars[k] = v
	}
	placement := Placement{Depth: 1, Character: id, Name: "item", Matrix: Identity(), Color: NoColor(), Born: 1}
	next := placement
	next.Born = 2
	chars[0] = &Character{ID: 0, Kind: "sprite", Frames: []Frame{{Display: []Placement{placement}}, {Display: []Placement{next}}}}
	copyMovie := *movie
	copyMovie.Characters = chars
	r, err := New(&copyMovie, seed)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestArchivedUpgradeLoadValues(t *testing.T) {
	movie := archivedMovie(t)
	cases := []struct {
		id            int
		flag          string
		before, after float64
	}{{209, "rockBonus", 20, 60}, {219, "rockBonus", 11, 33}, {312, "rockBonus", 11, 33}, {289, "diamondBonus", 600, 900}, {320, "diamondBonus", 602, 902}}
	for _, c := range cases {
		r := objectRuntime(t, movie, c.id, 1)
		if got := number(r.Get(r.Root, "item.ob.price")); got != c.before {
			t.Fatalf("item %d initial price=%v", c.id, got)
		}
		r.Set(r.Root, c.flag, float64(1))
		if err := r.Goto(r.Root, 2, false); err != nil {
			t.Fatal(err)
		}
		if got := number(r.Get(r.Root, "item.ob.price")); got != c.after {
			t.Fatalf("item %d upgraded price=%v", c.id, got)
		}
	}
	outcomes := map[string]bool{}
	for seed := uint64(1); seed <= 24; seed++ {
		r := objectRuntime(t, movie, 214, seed)
		r.Set(r.Root, "cloverBonus", float64(1))
		if err := r.Goto(r.Root, 2, false); err != nil {
			t.Fatal(err)
		}
		w := number(r.Get(r.Root, "item.ob.w2"))
		price := r.Get(r.Root, "item.ob.price")
		expected := any(float64(700))
		if w <= 2 {
			expected = "power"
		} else if w <= 4 {
			expected = "dynamite"
		}
		if !equal(price, expected) {
			t.Fatalf("clover roll=%v price=%v expected=%v", w, price, expected)
		}
		outcomes[String(price)] = true
	}
	if len(outcomes) != 3 {
		t.Fatalf("clover outcomes=%v", outcomes)
	}
}

func TestArchivedGrabBagPowerAndDynamiteDelivery(t *testing.T) {
	movie := archivedMovie(t)
	seen := map[string]bool{}
	for seed := uint64(1); seed <= 40 && len(seen) < 2; seed++ {
		r, err := New(movie, seed)
		if err != nil {
			t.Fatal(err)
		}
		startGame(t, r)
		ticks(t, r, 40)
		bag, _ := r.Get(r.Root, "T14").(*Node)
		if bag == nil {
			t.Fatal("grab bag missing")
		}
		price := r.Get(bag, "ob.price")
		kind, ok := price.(string)
		if !ok || seen[kind] {
			continue
		}
		for i := 1; i <= int(number(r.Get(r.Root, "things"))); i++ {
			target, _ := r.Get(r.Root, "T"+String(float64(i))).(*Node)
			target.Matrix.TX += 10000
		}
		swing, _ := r.Get(r.Root, "C").(*Node)
		if err = r.Goto(swing, 3, false); err != nil {
			t.Fatal(err)
		}
		cl, _ := r.Get(swing, "cl").(*Node)
		if err = r.Goto(cl, 2, false); err != nil {
			t.Fatal(err)
		}
		sensor, _ := r.Get(cl, "box").(*Node)
		box, _ := r.Get(bag, "ob.box").(*Node)
		a, b := r.Bounds(sensor), r.Bounds(box)
		bag.Matrix.TX += a.X - b.X
		bag.Matrix.TY += a.Y - b.Y
		ticks(t, r, 1)
		if bag.Frame != 2 || r.Get(cl, "price") != price {
			t.Fatalf("%s bag was not collected: frame=%d price=%v", kind, bag.Frame, r.Get(cl, "price"))
		}
		ticks(t, r, 150)
		switch kind {
		case "power":
			if number(r.Get(r.Root, "strength")) != 1 {
				t.Fatal("power bag did not set strength")
			}
		case "dynamite":
			if number(r.Get(r.Root, "stick")) != 1 {
				t.Fatal("dynamite bag did not add a stick")
			}
		default:
			t.Fatalf("unknown bag bonus %q", kind)
		}
		if number(r.Get(r.Root, "score")) != 0 {
			t.Fatalf("%s credited numeric score %v", kind, r.Get(r.Root, "score"))
		}
		seen[kind] = true
	}
	if len(seen) != 2 {
		t.Fatalf("missing seeded bag paths: %v", seen)
	}
}

func TestArchivedMovingAnimalCollection(t *testing.T) {
	r := archivedRuntime(t)
	startGame(t, r)
	ticks(t, r, 40)
	if err := r.Goto(r.Root, "L6_1", false); err != nil {
		t.Fatal(err)
	}
	target, _ := r.Get(r.Root, "T7").(*Node)
	animal, _ := r.Get(target, "ob").(*Node)
	first := animal.Frame
	ticks(t, r, 18)
	if animal.Frame == first || animal.Def.ID != 319 {
		t.Fatalf("animal did not animate: id=%d frame=%d", animal.Def.ID, animal.Frame)
	}
	price := number(r.Get(animal, "price"))
	for i := 1; i <= int(number(r.Get(r.Root, "things"))); i++ {
		n, _ := r.Get(r.Root, "T"+String(float64(i))).(*Node)
		n.Matrix.TX += 10000
	}
	swing, _ := r.Get(r.Root, "C").(*Node)
	if err := r.Goto(swing, 3, false); err != nil {
		t.Fatal(err)
	}
	cl, _ := r.Get(swing, "cl").(*Node)
	if err := r.Goto(cl, 2, false); err != nil {
		t.Fatal(err)
	}
	sensor, _ := r.Get(cl, "box").(*Node)
	box, _ := r.Get(animal, "box").(*Node)
	a, b := r.Bounds(sensor), r.Bounds(box)
	target.Matrix.TX += a.X - b.X
	target.Matrix.TY += a.Y - b.Y
	ticks(t, r, 1)
	if target.Frame != 2 || number(r.Get(cl, "object")) != 10 {
		t.Fatalf("animal not collected frame=%d object=%v", target.Frame, r.Get(cl, "object"))
	}
	ticks(t, r, 150)
	if number(r.Get(r.Root, "score")) != price {
		t.Fatalf("animal price=%v score=%v", price, r.Get(r.Root, "score"))
	}
}

func TestGotoFrameStopsUnlessFollowedByPlay(t *testing.T) {
	movie := &Movie{Characters: map[int]*Character{0: {ID: 0, Kind: "sprite", Frames: make([]Frame, 3)}}}
	for _, tc := range []struct {
		name string
		code []byte
		play bool
	}{
		{"stop", []byte{0x81, 2, 0, 1, 0, 0}, false},
		{"play", []byte{0x81, 2, 0, 1, 0, 0x06, 0}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := New(movie, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.run(r.Root, tc.code); err != nil {
				t.Fatal(err)
			}
			if r.Root.Frame != 2 || r.Root.Playing != tc.play {
				t.Fatalf("GotoFrame frame=%d playing=%v", r.Root.Frame, r.Root.Playing)
			}
			ticks(t, r, 1)
			want := 2
			if tc.play {
				want = 3
			}
			if r.Root.Frame != want {
				t.Fatalf("next tick frame=%d want=%d", r.Root.Frame, want)
			}
		})
	}
}

func TestArchivedShopPointerPricesAndPurchasesPersist(t *testing.T) {
	// Seed 32 makes all five original products available after this level.
	r, err := New(archivedMovie(t), 32)
	if err != nil {
		t.Fatal(err)
	}
	startGame(t, r)
	ticks(t, r, 40)
	timer := findCharacter(r.Root, 197)
	if timer == nil {
		t.Fatal("timer missing")
	}
	r.Set(r.Root, "score", float64(10000))
	r.Set(timer, "time", float64(1))
	ticks(t, r, 220)
	shop := findCharacter(r.Root, 133)
	if shop == nil {
		t.Fatal("shop missing")
	}
	items := []struct {
		id, button int
		x, y       float64
		bonus      string
		increment  float64
	}{
		{90, 89, 57, 257, "stick", 1},
		{96, 95, 134, 254, "minerStrength", 2},
		{102, 101, 196, 258, "cloverBonus", 1},
		{107, 106, 268, 260, "rockBonus", 1},
		{111, 110, 336, 265, "diamondBonus", 1},
	}
	type snapshot struct {
		n        *Node
		price    float64
		priceDis any
		frame    int
	}
	products := make([]snapshot, len(items))
	for i, item := range items {
		n := findCharacter(r.Root, item.id)
		if n == nil || n.Frame != 2 {
			t.Fatalf("product %d unavailable", item.id)
		}
		products[i] = snapshot{n, number(r.Get(n, "price")), r.Get(n, "priceDis"), 2}
	}
	check := func() {
		t.Helper()
		if r.Root.Frame != 130 || r.Root.Playing || shop.Frame != 18 || shop.Playing {
			t.Fatalf("shop timeline root=%d/%v shop=%d/%v", r.Root.Frame, r.Root.Playing, shop.Frame, shop.Playing)
		}
		for _, p := range products {
			n := findCharacter(r.Root, p.n.Def.ID)
			if n == nil {
				t.Fatalf("product %d disappeared from shop", p.n.Def.ID)
			}
			if n != p.n || n.Frame != p.frame || n.Playing || number(r.Get(n, "price")) != p.price || r.Get(n, "priceDis") != p.priceDis {
				t.Fatalf("product %d restarted: frame=%d playing=%v price=%v expected=%v", p.n.Def.ID, n.Frame, n.Playing, r.Get(n, "price"), p.price)
			}
			if p.frame == 11 && len(n.Children) != 0 {
				t.Fatalf("sold product %d reappeared", n.Def.ID)
			}
		}
	}
	pointerTicks := func(x, y float64, down bool, count int) {
		t.Helper()
		for i := 0; i < count; i++ {
			if err := r.Pointer(x, y, down); err != nil {
				t.Fatal(err)
			}
			ticks(t, r, 1)
			check()
		}
	}
	check()
	for i, item := range items {
		buy := findCharacter(r.Root, item.button)
		pointerTicks(item.x, item.y, false, 150)
		if r.hover != buy {
			t.Fatalf("pointer missed product %d", item.id)
		}
		score := number(r.Get(r.Root, "score"))
		bonus := number(r.Get(r.Root, item.bonus))
		pointerTicks(item.x, item.y, true, 150)
		if number(r.Get(r.Root, "score")) != score {
			t.Fatalf("product %d purchased before release", item.id)
		}
		products[i].frame = 11
		pointerTicks(item.x, item.y, false, 150)
		if number(r.Get(r.Root, "score")) != score-products[i].price || number(r.Get(r.Root, item.bonus)) != bonus+item.increment {
			t.Fatalf("product %d purchase score=%v bonus=%v", item.id, r.Get(r.Root, "score"), r.Get(r.Root, item.bonus))
		}
		// Clicking the same sold-out slot cannot buy the item again.
		pointerTicks(item.x, item.y, true, 150)
		pointerTicks(item.x, item.y, false, 150)
		if number(r.Get(r.Root, "score")) != score-products[i].price || number(r.Get(r.Root, item.bonus)) != bonus+item.increment {
			t.Fatalf("sold product %d purchased twice", item.id)
		}
		pointerTicks(450, 200, false, 150)
	}
}
