package flash

import "testing"

type coopScoreShopItem struct {
	id, button int
	x, y       float64
}

func coopScoreShopItems() []coopScoreShopItem {
	return []coopScoreShopItem{
		{id: 90, button: 89, x: 57, y: 257},
		{id: 96, button: 95, x: 134, y: 254},
		{id: 102, button: 101, x: 196, y: 258},
		{id: 107, button: 106, x: 268, y: 260},
		{id: 111, button: 110, x: 336, y: 265},
	}
}

func coopScoreBonus(t *testing.T, r *Runtime, player int, score any) {
	t.Helper()
	name := "bonus"
	if player == 2 {
		name = "bonus2"
	}
	bonus := coopNode(t, r, name)
	r.Set(bonus, "bscore", score)
	if err := r.Goto(bonus, 2, true); err != nil {
		t.Fatal(err)
	}
	// The archived bonus action credits bscore on its original frame 32.
	ticks(t, r, 32)
}

func coopScoreShopRuntime(t *testing.T) *Runtime {
	t.Helper()
	r, err := New(archivedMovie(t), 32)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetPlayers(2); err != nil {
		t.Fatal(err)
	}
	startGame(t, r)
	ticks(t, r, 40)
	coopNode(t, r, "C").Playing = false
	coopNode(t, r, "C2").Playing = false
	// Keep seed 32's five-item shop roll while arranging unequal balances for
	// the purchase assertions; bonus ownership is covered by the test above.
	r.Set(r.Root, "score", float64(10000))
	r.coopScores = [2]float64{9999, 1}
	timer := findCharacter(r.Root, 197)
	if timer == nil {
		t.Fatal("timer missing")
	}
	r.Set(timer, "time", float64(1))
	ticks(t, r, 220)
	if r.Root.Frame != 130 || r.Root.Playing {
		t.Fatalf("shop frame=%d playing=%v", r.Root.Frame, r.Root.Playing)
	}
	return r
}

func coopScorePointerTicks(t *testing.T, r *Runtime, x, y float64, down bool) {
	t.Helper()
	if err := r.Pointer(x, y, down); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 1)
}

func assertCoopScores(t *testing.T, r *Runtime, want [2]float64) {
	t.Helper()
	if got := r.PlayerScores(); got != want {
		t.Fatalf("player scores=%v want %v", got, want)
	}
	if got := number(r.Root.Get("score")); got != want[0]+want[1] {
		t.Fatalf("root score=%v want %v", got, want[0]+want[1])
	}
}

func TestCooperativeBonusScoresHaveOwnerAndOriginalCreditFrame(t *testing.T) {
	r := cooperativeRuntime(t)
	coopNode(t, r, "C").Playing = false
	coopNode(t, r, "C2").Playing = false
	b1, b2 := coopNode(t, r, "bonus"), coopNode(t, r, "bonus2")

	r.Set(b1, "bscore", float64(41))
	if err := r.Goto(b1, 2, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 29)
	assertCoopScores(t, r, [2]float64{0, 0})
	ticks(t, r, 1)
	assertCoopScores(t, r, [2]float64{41, 0})

	r.Set(b2, "bscore", float64(73))
	if err := r.Goto(b2, 2, true); err != nil {
		t.Fatal(err)
	}
	ticks(t, r, 29)
	assertCoopScores(t, r, [2]float64{41, 0})
	ticks(t, r, 1)
	assertCoopScores(t, r, [2]float64{41, 73})

	for _, reward := range []string{"power", "dynamite"} {
		before := r.PlayerScores()
		r.Set(b2, "bscore", reward)
		if err := r.Goto(b2, 2, true); err != nil {
			t.Fatal(err)
		}
		ticks(t, r, 90)
		if got := r.PlayerScores(); got != before {
			t.Fatalf("%s changed player scores from %v to %v", reward, before, got)
		}
		assertCoopScores(t, r, before)
	}
}

func TestCooperativeShopPointerPurchasesSplitEachPrice(t *testing.T) {
	r := coopScoreShopRuntime(t)
	items := coopScoreShopItems()
	assertCoopScores(t, r, [2]float64{9999, 1})

	// Make the first real product odd-priced. Player two has less than half,
	// but the shared total still covers it; the resulting balance may be -49.5.
	first := findCharacter(r.Root, items[0].id)
	if first == nil || first.Frame != 2 {
		t.Fatal("first product unavailable")
	}
	r.Set(first, "price", float64(101))

	for i, item := range items {
		product := findCharacter(r.Root, item.id)
		buy := findCharacter(r.Root, item.button)
		if product == nil || buy == nil || product.Frame != 2 {
			t.Fatalf("product %d unavailable", item.id)
		}
		price := number(r.Get(product, "price"))
		before := r.PlayerScores()
		rootBefore := number(r.Root.Get("score"))
		if i == 0 && (before[0] == before[1] || before[1] >= price/2) {
			t.Fatalf("unequal low balance was not exercised: scores=%v price=%v", before, price)
		}

		coopScorePointerTicks(t, r, item.x, item.y, false)
		if r.hover != buy {
			t.Fatalf("pointer missed product %d", item.id)
		}
		coopScorePointerTicks(t, r, item.x, item.y, true)
		if number(r.Root.Get("score")) != rootBefore || r.PlayerScores() != before {
			t.Fatalf("product %d purchased before release", item.id)
		}
		coopScorePointerTicks(t, r, item.x, item.y, false)
		want := [2]float64{before[0] - price/2, before[1] - price/2}
		assertCoopScores(t, r, want)
		if i == 0 && want[1] != -49.5 {
			t.Fatalf("odd purchase truncated player two balance: %v", want[1])
		}

		// Clicking a sold slot again must not deduct another half-price share.
		coopScorePointerTicks(t, r, item.x, item.y, true)
		coopScorePointerTicks(t, r, item.x, item.y, false)
		assertCoopScores(t, r, want)
		coopScorePointerTicks(t, r, 450, 200, false)
	}
}
