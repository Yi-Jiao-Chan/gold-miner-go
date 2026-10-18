// Gold Miner runs the archived game's display list and AVM1 instructions in Go.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"bytes"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"goldminer/assets"
	"goldminer/internal/audiofx"
	"goldminer/internal/flash"
	"goldminer/internal/render"
	"goldminer/internal/savegame"
)

type game struct {
	seed                            uint64
	statusFile                      string
	rt                              *flash.Runtime
	renderer                        *render.Renderer
	audio                           *audio.Context
	sounds                          map[int][]byte
	players                         []*audio.Player
	active                          map[int]*audio.Player
	movie                           *flash.Movie
	snapshot                        string
	snapshotTicks                   int
	ready, done                     bool
	clock                           frameClock
	screenWidth, screenHeight       int
	store                           *savegame.Store
	record                          savegame.Record
	records                         []savegame.Record
	ui, uiPressed, notice, deleteID string
	deleteFrom                      string
	page, resumeLeft                int
	paused, shopSaved, historySaved bool
}

func (g *game) Update() error {
	if g.done {
		return ebiten.Termination
	}
	if g.snapshot != "" {
		if !g.ready {
			for i := 0; i < g.snapshotTicks; i++ {
				if err := g.rt.Tick(); err != nil {
					return err
				}
			}
			g.ready = true
		}
		return nil
	}
	if ebiten.IsWindowBeingClosed() {
		g.saveOrNotice(false)
		return ebiten.Termination
	}
	in := controls{Escape: inpututil.IsKeyJustPressed(ebiten.KeyEscape), Down: ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft), Pressed: inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft), Released: inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)}
	in.Keys = pressedPlayerKeys(g.rt.Players(), inpututil.IsKeyJustPressed)
	x, y := ebiten.CursorPosition()
	v := render.Viewport(g.screenWidth, g.screenHeight)
	in.X = (float64(x) - v.TX) / v.A
	in.Y = (float64(y) - v.TY) / v.D
	if err := g.step(in); err != nil {
		return err
	}
	g.writeStatus()
	if g.paused || g.ui != "" {
		return nil
	}
	for _, id := range g.rt.DrainSounds() {
		pcm, ok := g.sounds[id]
		if !ok {
			continue
		}
		source := id
		if variant, ok := g.movie.SoundVariants[id]; ok {
			source = variant.Source
			if variant.NoMultiple && g.active[source] != nil && g.active[source].IsPlaying() {
				continue
			}
		}
		p := g.audio.NewPlayerFromBytes(pcm)
		p.Play()
		g.active[source] = p
		g.players = append(g.players, p)
	}
	live := g.players[:0]
	for _, p := range g.players {
		if p.IsPlaying() {
			live = append(live, p)
		} else {
			p.Close()
			for id, active := range g.active {
				if active == p {
					delete(g.active, id)
				}
			}
		}
	}
	g.players = live
	return nil
}
func (g *game) Draw(screen *ebiten.Image) {
	g.renderer.Draw(screen, g.rt)
	g.drawUI(screen)
	if g.snapshot != "" && g.ready && !g.done {
		im := image.NewRGBA(screen.Bounds())
		screen.ReadPixels(im.Pix)
		f, err := os.Create(g.snapshot)
		if err != nil {
			log.Fatal(err)
		}
		if err = png.Encode(f, im); err != nil {
			log.Fatal(err)
		}
		if err = f.Close(); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("snapshot=%s rootFrame=%d score=%v goal=%v level=%v\n", g.snapshot, g.rt.Root.Frame, g.rt.Get(g.rt.Root, "score"), g.rt.Get(g.rt.Root, "goal"), g.rt.Get(g.rt.Root, "level"))
		g.done = true
	}
}
func (g *game) Layout(width, height int) (int, int) {
	dpi := ebiten.Monitor().DeviceScaleFactor()
	g.screenWidth = max(1, int(float64(width)*dpi))
	g.screenHeight = max(1, int(float64(height)*dpi))
	return g.screenWidth, g.screenHeight
}
func main() {
	seed := flag.Uint64("seed", uint64(time.Now().UnixNano()), "random seed for repeatable game runs")
	scale := flag.Int("scale", 2, "initial window scale (1 = original 550 x 400)")
	snapshot := flag.String("snapshot", "", "save a PNG and exit (validation)")
	ticks := flag.Int("ticks", 0, "frames to advance before snapshot")
	frame := flag.Int("frame", 0, "seek original root frame (validation)")
	statusFile := flag.String("status-file", "", "write runtime status for desktop validation")
	saveDir := flag.String("save-dir", "", "override save directory (validation)")
	players := flag.Int("players", 1, "validation player count: 1 or 2")
	layout := flag.String("layout", "", "select an original layout, e.g. L1_1 (validation)")
	flag.Parse()
	if *scale < 1 || *scale > 4 {
		log.Fatal("scale must be between 1 and 4")
	}
	raw, err := fs.ReadFile(assets.Files, "movie.json")
	if err != nil {
		log.Fatal(err)
	}
	var movie flash.Movie
	if err = json.Unmarshal(raw, &movie); err != nil {
		log.Fatal(err)
	}
	rt, err := flash.New(&movie, *seed)
	if err != nil {
		log.Fatal(err)
	}
	if *layout != "" {
		if _, ok := movie.Characters[0].Labels[*layout]; !ok {
			log.Fatalf("unknown original layout %q", *layout)
		}
		for k, v := range map[string]any{"minerStrength": float64(10), "total": float64(10), "time": float64(60), "goalAddOn": float64(0), "goal": float64(375), "level": float64(0), "stick": float64(0), "score": float64(0), "levelDis": float64(0)} {
			rt.Set(rt.Root, k, v)
		}
		if err = rt.Goto(rt.Root, 15, false); err != nil {
			log.Fatal(err)
		}
		// Reach the requested group through the original goal progression scripts.
		var group, variant int
		if _, err := fmt.Sscanf(*layout, "L%d_%d", &group, &variant); err != nil {
			log.Fatal(err)
		}
		if err = rt.Goto(rt.Root, "L1_1", false); err != nil {
			log.Fatal(err)
		}
		for level := 2; level <= group; level++ {
			if err = rt.Goto(rt.Root, 52, false); err != nil {
				log.Fatal(err)
			}
			if err = rt.Goto(rt.Root, fmt.Sprintf("L%d_1", level), false); err != nil {
				log.Fatal(err)
			}
		}
		if err = rt.Goto(rt.Root, *layout, false); err != nil {
			log.Fatal(err)
		}
	} else if *frame > 0 {
		if err = rt.Goto(rt.Root, *frame, false); err != nil {
			log.Fatal(err)
		}
	}
	if err = rt.SetPlayers(*players); err != nil {
		log.Fatal(err)
	}
	renderer, err := render.New(assets.Files, &movie)
	if err != nil {
		log.Fatal(err)
	}
	g := &game{seed: *seed, statusFile: *statusFile, rt: rt, renderer: renderer, snapshot: *snapshot, snapshotTicks: *ticks, sounds: map[int][]byte{}, active: map[int]*audio.Player{}, movie: &movie}
	if *snapshot == "" {
		dir := *saveDir
		if dir == "" {
			base, e := os.UserConfigDir()
			if e != nil {
				log.Fatal(e)
			}
			dir = filepath.Join(base, "GoldMiner", "saves")
		}
		g.store, err = savegame.Open(dir)
		if err != nil {
			log.Fatal("无法打开存档目录：", err)
		}
		g.ui = "home"
		g.refreshSaves()
		g.audio = audio.NewContext(44100)
		for id, path := range movie.Sounds {
			b, err := fs.ReadFile(assets.Files, path)
			if err != nil {
				log.Fatal(err)
			}
			stream, err := wav.DecodeWithSampleRate(44100, bytes.NewReader(b))
			if err != nil {
				log.Fatal(err)
			}
			// The original WAV files are small; decode once, then reuse native players.
			pcm, err := io.ReadAll(stream)
			if err != nil {
				log.Fatal(err)
			}
			g.sounds[id] = pcm
		}
		for id, variant := range movie.SoundVariants {
			g.sounds[id] = audiofx.Prepare(g.sounds[variant.Source], variant)
		}
	}
	iconData, err := fs.ReadFile(assets.Files, "icons/goldminer.png")
	if err != nil {
		log.Fatal(err)
	}
	windowIcon, err := png.Decode(bytes.NewReader(iconData))
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowIcon([]image.Image{windowIcon})
	ebiten.SetWindowSize(550**scale, 400**scale)
	ebiten.SetWindowTitle("黄金矿工")
	ebiten.SetWindowClosingHandled(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(inputTPS)
	if err = ebiten.RunGameWithOptions(g, &ebiten.RunGameOptions{X11ClassName: "GoldMiner", X11InstanceName: "goldminer"}); err != nil {
		log.Fatal(err)
	}
}
