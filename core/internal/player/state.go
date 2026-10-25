package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

// stateVersion is bumped with a migration in loadState whenever the saved
// format changes.
const stateVersion = 2

type savedState struct {
	Version  int        `json:"version"`
	Queue    QueueState `json:"queue"`
	Position float64    `json:"position"`
	Volume   float64    `json:"volume"`
	Muted    bool       `json:"muted,omitempty"`
	// Feed marks a shuffle over whole libraries, rebuilt from the origin.
	Feed bool `json:"feed,omitempty"`
	// Continue is continuing turned on for this queue (Player.Continue).
	Continue string    `json:"continue,omitempty"`
	SavedAt  time.Time `json:"savedAt"`
}

func loadState(path string) (savedState, error) {
	var st savedState
	data, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("player state is corrupt: %w", err)
	}
	if st.Version > stateVersion {
		return st, fmt.Errorf("player state version %d is newer than this dplayer (%d)", st.Version, stateVersion)
	}
	return st, nil
}

func writeState(path string, st savedState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, 0o600)
}

// restore loads the saved queue stopped, at its saved position, so resume
// can pick up where playback was. A state file that can't be read is moved
// aside rather than overwritten.
func (p *Player) restore() {
	if p.cfg.StatePath == "" {
		return
	}
	st, err := loadState(p.cfg.StatePath)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		aside := p.cfg.StatePath + ".unreadable"
		log.Warnf("%v; moved to %s", err, aside)
		_ = os.Rename(p.cfg.StatePath, aside)
		return
	}
	p.queue.Restore(st.Queue)
	p.pos, p.savedPos = max(st.Position, 0), st.Position
	if st.Volume >= 0 {
		p.volume = min(st.Volume, p.opts.VolumeMax)
	}
	p.muted = st.Muted
	p.refeed = st.Feed
	if st.Continue == ContinueRandom || st.Continue == ContinueSimilar {
		p.contMode = st.Continue
	}
}

func (p *Player) markDirty() {
	if p.cfg.StatePath == "" {
		return
	}
	p.dirty = true
	if p.saveTimer == nil {
		p.saveTimer = time.After(saveDelay)
	}
}

func (p *Player) save() {
	p.saveTimer = nil
	if !p.dirty {
		return
	}
	p.dirty = false
	p.savedPos = p.pos
	st := savedState{Version: stateVersion, Queue: p.queue.State(), Position: p.pos, Volume: p.volume, Muted: p.muted,
		Feed: p.feed != nil || p.refeed, Continue: p.contMode, SavedAt: time.Now()}
	if err := writeState(p.cfg.StatePath, st); err != nil {
		log.Warnf("save player state: %v", err)
	}
}
