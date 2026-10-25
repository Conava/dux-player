// Package sysvol is the default audio output's volume and mute, through
// WirePlumber's wpctl, for the "system" volume target. Changes made
// elsewhere arrive through `pactl subscribe` while someone watches.
package sysvol

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
)

const (
	sink        = "@DEFAULT_AUDIO_SINK@"
	callTimeout = 3 * time.Second
	maxOutput   = 4 << 10
	// settle merges the burst of events one volume change makes.
	settle = 50 * time.Millisecond
	// rewatch is how long a watcher whose pactl ended waits to start it again.
	rewatch = 10 * time.Second
)

// Mixer runs wpctl; pactl, when installed, reports changes.
type Mixer struct {
	wpctl, pactl string
}

// Find returns the mixer, or nil without wpctl.
func Find() *Mixer {
	w, err := exec.LookPath("wpctl")
	if err != nil {
		return nil
	}
	p, _ := exec.LookPath("pactl")
	return &Mixer{wpctl: w, pactl: p}
}

// capped keeps the first maxOutput bytes written to it.
type capped struct{ bytes.Buffer }

func (c *capped) Write(b []byte) (int, error) {
	if room := maxOutput - c.Len(); room > 0 {
		c.Buffer.Write(b[:min(len(b), room)])
	}
	return len(b), nil
}

func (m *Mixer) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var out, errOut capped
	cmd := exec.CommandContext(ctx, m.wpctl, args...)
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errOut.String()); msg != "" {
			return "", fmt.Errorf("wpctl %s: %s", args[0], msg)
		}
		return "", fmt.Errorf("wpctl %s: %w", args[0], err)
	}
	return out.String(), nil
}

// Get returns the volume in percent and whether the output is muted.
func (m *Mixer) Get(ctx context.Context) (float64, bool, error) {
	out, err := m.run(ctx, "get-volume", sink)
	if err != nil {
		return 0, false, err
	}
	return parse(out)
}

// parse reads wpctl's "Volume: 0.40" or "Volume: 0.40 [MUTED]".
func parse(out string) (float64, bool, error) {
	fields := strings.Fields(out)
	if len(fields) < 2 || fields[0] != "Volume:" {
		return 0, false, errors.New("wpctl get-volume answered something unexpected")
	}
	v, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || v < 0 || v > 10 {
		return 0, false, errors.New("wpctl get-volume answered something unexpected")
	}
	return v * 100, strings.Contains(out, "[MUTED]"), nil
}

func (m *Mixer) SetVolume(ctx context.Context, percent float64) error {
	_, err := m.run(ctx, "set-volume", sink, strconv.FormatFloat(percent/100, 'f', 3, 64))
	return err
}

func (m *Mixer) SetMute(ctx context.Context, muted bool) error {
	state := "0"
	if muted {
		state = "1"
	}
	_, err := m.run(ctx, "set-mute", sink, state)
	return err
}

// Watch calls changed after the default output or its volume may have
// changed, until ctx ends. Without pactl it returns at once.
func (m *Mixer) Watch(ctx context.Context, changed func()) {
	if m.pactl == "" {
		return
	}
	for {
		err := m.watchOnce(ctx, changed)
		if ctx.Err() != nil {
			return
		}
		log.Warnf("watching the system volume: %v; trying again in %s", err, rewatch)
		select {
		case <-ctx.Done():
			return
		case <-time.After(rewatch):
		}
	}
}

func (m *Mixer) watchOnce(ctx context.Context, changed func()) error {
	cmd := exec.CommandContext(ctx, m.pactl, "subscribe")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var timer *time.Timer
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		// "Event 'change' on sink #57"; server events cover a new default.
		line := sc.Text()
		if !strings.Contains(line, " on sink #") && !strings.Contains(line, " on server") {
			continue
		}
		if timer == nil {
			timer = time.AfterFunc(settle, changed)
		} else {
			timer.Reset(settle)
		}
	}
	if timer != nil {
		timer.Stop()
	}
	err = cmd.Wait()
	if err == nil {
		err = errors.New("pactl subscribe ended")
	}
	return err
}
