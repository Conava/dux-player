package main

import (
	"sync"
	"time"

	"github.com/Conava/dux-player/core/internal/discord"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/player"
)

// discordSeekDrift is how far playback may stray from the start time Discord
// shows before the card is sent again.
const discordSeekDrift = 2 * time.Second

// presence mirrors the playing track to Discord while the setting is on.
type presence struct {
	mu     sync.Mutex
	client *discord.Client
	closed bool
	last   player.Status
	start  time.Time // start time of the card Discord shows; zero when none
}

// enable creates or closes the client to match the setting.
func (p *presence) enable(on bool) {
	p.mu.Lock()
	var old *discord.Client
	switch {
	case on && p.client == nil && !p.closed:
		p.client = discord.New()
		p.show(p.last.Position)
	case !on && p.client != nil:
		old, p.client, p.start = p.client, nil, time.Time{}
	}
	p.mu.Unlock()
	if old != nil {
		old.Close()
	}
}

func (p *presence) status(st player.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prev := p.last
	p.last = st
	if playing(prev) == playing(st) && (!playing(st) || sameTrack(prev, st)) {
		return
	}
	p.show(st.Position)
}

func playing(st player.Status) bool { return st.State == player.StatePlaying && st.Track != nil }

func sameTrack(a, b player.Status) bool {
	return a.Track != nil && b.Track != nil && a.Track.Library == b.Track.Library && a.Track.ID == b.Track.ID
}

// position resends the card when a seek moved the implied start time.
func (p *presence) position(pos player.Position) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil || !pos.Playing || pos.Speed != 1 || p.start.IsZero() {
		return
	}
	start := time.UnixMilli(pos.At).Add(-time.Duration(pos.Position * float64(time.Second)))
	if d := start.Sub(p.start); d < discordSeekDrift && d > -discordSeekDrift {
		return
	}
	p.show(pos.Position)
}

// show sends or clears the card for the last status. Callers hold mu.
func (p *presence) show(position float64) {
	if p.client == nil {
		return
	}
	st := p.last
	if st.State != player.StatePlaying || st.Track == nil {
		p.client.Clear()
		p.start = time.Time{}
		return
	}
	p.start = time.Now().Add(-time.Duration(position * float64(time.Second)))
	p.client.Set(discord.Activity{Title: st.Track.Title, Artist: st.Track.Artist, Album: st.Track.Album, Start: p.start})
}

func (p *presence) close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.enable(false)
}

func init() {
	p := &presence{}
	addFeature(feature{
		register: func(*ipc.Registry, *daemon) {},
		start: func(d *daemon) error {
			p.enable(d.Settings.Get().DiscordPresence)
			d.Bus.On(d.Ctx, func(topic string, data any) {
				switch v := data.(type) {
				case player.Status:
					p.status(v)
				case player.Position:
					p.position(v)
				default:
					if topic == ipc.TopicSettings {
						p.enable(d.Settings.Get().DiscordPresence)
					}
				}
			}, ipc.TopicSettings, player.TopicStatus, player.TopicPosition)
			return nil
		},
		stop: p.close,
	})
}
