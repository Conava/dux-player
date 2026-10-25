package devices

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/settings"
)

// The equalizer is one more `pipewire -c` child after mpv (filter-chain sink
// dplayer_eq), so mpv's track changes never reset its filters. PipeWire
// biquads restart from zero state (a click) on any coefficient change, so the
// graph holds two copies of the bands: a change goes to the muted copy, which
// settles unheard, then the mixer crossfades to it.
const (
	eqNode = "dplayer_eq"
	// EQDevice is what mpv plays to while the equalizer is on.
	EQDevice = "pipewire/" + eqNode
	// eqSettle outlasts the transient of the narrowest band (31 Hz, about
	// 15 ms time constant) many times over.
	eqSettle = 150 * time.Millisecond
	// The crossfade takes eqFadeSteps steps of about one graph cycle each.
	eqFadeSteps = 16
	eqStepEvery = 20 * time.Millisecond
	// Q 1.41 is one octave wide, like the bands' spacing.
	eqQ = 1.41
)

var eqChains = [2]string{"a", "b"}

// EQ is the equalizer as set: on or off, and the gain in dB per
// settings.EQBands entry.
type EQ struct {
	On    bool
	Gains [10]float64
}

// preamp keeps boosts from clipping: the largest boost, as a cut.
func preamp(g [10]float64) float64 {
	peak := 0.0
	for _, v := range g {
		peak = max(peak, v)
	}
	return math.Pow(10, -peak/20)
}

// eqConf targets sink, or the system default when sink is empty. wait keeps
// the stream unlinked instead of moving it to the default output when sink
// goes away. Both copies start at g, a heard.
func eqConf(sink string, wait bool, g [10]float64) string {
	var nodes, links strings.Builder
	nodes.WriteString(`          { type = builtin label = copy name = in }
          { type = builtin label = mixer name = mix control = { "Gain 1" = 1.0 "Gain 2" = 0.0 } }
`)
	for n, c := range eqChains {
		fmt.Fprintf(&nodes, `          { type = builtin label = linear name = %spre control = { "Mult" = %s "Add" = 0.0 } }`+"\n", c, secs(preamp(g)))
		fmt.Fprintf(&links, `          { output = "in:Out" input = "%spre:In" }`+"\n", c)
		prev := c + "pre"
		for i, f := range settings.EQBands {
			fmt.Fprintf(&nodes, `          { type = builtin label = bq_peaking name = %s%d control = { "Freq" = %d.0 "Q" = %v "Gain" = %s } }`+"\n", c, i, f, eqQ, secs(g[i]))
			fmt.Fprintf(&links, `          { output = "%s:Out" input = "%s%d:In" }`+"\n", prev, c, i)
			prev = fmt.Sprintf("%s%d", c, i)
		}
		fmt.Fprintf(&links, `          { output = "%s:Out" input = "mix:In %d" }`+"\n", prev, n+1)
	}
	play := `media.class = "Stream/Output/Audio/Internal" stream.dont-remix = true state.restore-props = false`
	if sink != "" {
		play += ` target.object = "` + sink + `" node.dont-move = true`
	}
	if wait {
		play += ` node.dont-fallback = true node.linger = true`
	}
	return confHead + `  { name = libpipewire-module-filter-chain
    args = {
      node.description = "Dux Player equalizer"
      audio.channels = 2
      audio.position = [ FL FR ]
      filter.graph = {
        nodes = [
` + nodes.String() + `        ]
        links = [
` + links.String() + `        ]
        inputs = [ "in:In" ]
        outputs = [ "mix:Out" ]
      }
      capture.props = { node.name = "` + eqNode + `" media.class = "Audio/Sink/Internal" state.restore-props = false }
      playback.props = { node.name = "` + eqNode + `.play" ` + play + ` }
    }
  }
]
`
}

// chainParams sets copy c (0 or 1) to g.
func chainParams(c int, g [10]float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{ params = [ "%spre:Mult" %s`, eqChains[c], secs(preamp(g)))
	for i, v := range g {
		fmt.Fprintf(&b, ` "%s%d:Gain" %s`, eqChains[c], i, secs(v))
	}
	b.WriteString(" ] }")
	return b.String()
}

// mixParams is step k of eqFadeSteps from copy from to the other one.
func mixParams(from, k int) string {
	to := float64(k) / eqFadeSteps
	gains := [2]float64{}
	gains[from], gains[1-from] = 1-to, to
	return fmt.Sprintf(`{ params = [ "mix:Gain 1" %s "mix:Gain 2" %s ] }`, secs(gains[0]), secs(gains[1]))
}

// eqChild is the running equalizer, retuned live by its tuner goroutine.
type eqChild struct {
	c     *child
	sink  string
	wait  bool
	gains [10]float64
	want  chan [10]float64
	quit  chan struct{}
	done  chan struct{}
}

func startEQ(ctx context.Context, dir, sink string, wait bool, g [10]float64) (*eqChild, error) {
	c, err := startChild(dir, "eq", eqConf(sink, wait, g))
	if err != nil {
		return nil, err
	}
	if err := waitNode(ctx, eqNode); err != nil {
		c.close()
		return nil, err
	}
	e := &eqChild{c: c, sink: sink, wait: wait, gains: g, want: make(chan [10]float64, 1), quit: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(e.done)
		id := ""
		tune(e.want, e.quit, func(params string) error { return e.set(&id, params) }, e.c.gen.Load, sleep)
	}()
	return e, nil
}

// retune moves the gains to g; the newest g wins.
func (e *eqChild) retune(g [10]float64) {
	if g == e.gains {
		return
	}
	e.gains = g
	// The file is what a restarted child reads.
	if err := e.c.rewrite(eqConf(e.sink, e.wait, g)); err != nil {
		log.Warnf("equalizer: %v", err)
	}
	select {
	case <-e.want:
	default:
	}
	e.want <- g
}

// sleep waits d or until quit; false means quit.
func sleep(quit <-chan struct{}, d time.Duration) bool {
	select {
	case <-quit:
		return false
	case <-time.After(d):
		return true
	}
}

// tune applies each wanted g to the muted copy, lets it settle and
// crossfades to it. A g that arrives meanwhile follows once that is done.
// gen changes when the child restarts.
func tune(want <-chan [10]float64, quit <-chan struct{}, set func(string) error, gen func() uint64, wait func(<-chan struct{}, time.Duration) bool) {
	heard, seen := 0, gen()
	for {
		var g [10]float64
		select {
		case <-quit:
			return
		case g = <-want:
		}
		// A restarted child came up from the config file: copy a heard.
		if n := gen(); n != seen {
			heard, seen = 0, n
		}
		muted := 1 - heard
		if err := set(chainParams(muted, g)); err != nil {
			log.Warnf("equalizer: %v", err)
			continue
		}
		if !wait(quit, eqSettle) {
			return
		}
		for k := 1; k <= eqFadeSteps; k++ {
			if err := set(mixParams(heard, k)); err != nil {
				log.Warnf("equalizer: %v", err)
			}
			if !wait(quit, eqStepEvery) {
				return
			}
		}
		heard = muted
	}
}

// set looks the node id up once per child (it changes when the child
// restarts) and sets params.
func (e *eqChild) set(id *string, params string) error {
	ctx := context.Background()
	for range 2 {
		if *id == "" {
			var err error
			if *id, err = nodeID(ctx, eqNode); err != nil {
				return err
			}
		}
		if setParams(ctx, *id, params) == nil {
			return nil
		}
		*id = ""
	}
	return fmt.Errorf("couldn't set the bands on %s", eqNode)
}

func (e *eqChild) close() {
	close(e.quit)
	<-e.done
	e.c.close()
}
