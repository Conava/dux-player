package player

import (
	"context"
	"errors"
	"fmt"

	"github.com/Conava/dux-player/core/internal/source"
)

const (
	// refsHead is how many refs from the played one on resolve before playback
	// starts: that track and the next few.
	refsHead = 5
	// refsBatch is how many refs resolve per round in the background; each
	// round lands in the queue (and the queue event) at once.
	refsBatch = 50
)

// ResolveRefs turns refs into tracks; refs that don't resolve are left out
// and counted.
type ResolveRefs func(ctx context.Context, refs []source.Ref) (tracks []source.Track, dropped int)

// PlayRefs plays a list of refs from index without waiting for all of them:
// the played ref and the next few resolve first and start, the rest resolve
// in the background (after the played one, then before it) and join the queue
// as they arrive, unless a newer queue replaced it. Refs that don't resolve
// are left out with one notice. shuffle plays the list shuffled instead.
func (p *Player) PlayRefs(ctx context.Context, refs []source.Ref, index int, origin Origin, shuffle bool, resolve ResolveRefs) error {
	if index < 0 || index >= len(refs) {
		return fmt.Errorf("index %d is out of range for %d tracks", index, len(refs))
	}
	end := min(index+refsHead, len(refs))
	head, dropped := resolve(ctx, refs[index:end])
	// the played ref may be the one that failed: the next that resolves plays
	for len(head) == 0 && end < len(refs) {
		next := min(end+refsHead, len(refs))
		var d int
		head, d = resolve(ctx, refs[end:next])
		dropped += d
		end = next
	}
	if len(head) == 0 {
		return errors.New("none of these tracks could be found")
	}
	var gen uint64
	err := p.doPlay(ctx, func(ctx context.Context) error {
		p.queue.SetShuffle(shuffle)
		p.newQueue()
		gen = p.queueGen
		start := 0
		if shuffle {
			start = -1
		}
		p.queue.Replace(head, start, origin)
		return p.loadCurrent(ctx, 0, true)
	})
	if err != nil {
		return err
	}
	before, after := refs[:index], refs[end:]
	if len(before)+len(after) == 0 {
		p.reportDropped(dropped)
		return nil
	}
	go p.resolveRest(gen, before, after, dropped, resolve)
	return nil
}

func (p *Player) resolveRest(gen uint64, before, after []source.Ref, dropped int, resolve ResolveRefs) {
	ctx := p.runCtx
	for len(after) > 0 {
		n := min(refsBatch, len(after))
		tracks, d := resolve(ctx, after[:n])
		dropped += d
		after = after[n:]
		if !p.fill(ctx, gen, nil, tracks) {
			return
		}
	}
	// before the played one, last first, so the queue grows backwards from it
	for len(before) > 0 {
		n := min(refsBatch, len(before))
		tracks, d := resolve(ctx, before[len(before)-n:])
		dropped += d
		before = before[:len(before)-n]
		if !p.fill(ctx, gen, tracks, nil) {
			return
		}
	}
	_ = p.do(ctx, func(context.Context) error {
		if p.queueGen == gen {
			p.reportDropped(dropped)
		}
		return nil
	})
}

// fill adds resolved tracks to queue gen: before the played track (in order)
// and at the end. false once that queue is gone.
func (p *Player) fill(ctx context.Context, gen uint64, before, after []source.Track) bool {
	alive := false
	err := p.do(ctx, func(ctx context.Context) error {
		if p.queueGen != gen {
			return nil
		}
		alive = true
		if len(before)+len(after) == 0 {
			return nil
		}
		p.queue.Grow(before, after)
		p.syncNext(ctx)
		p.publishQueue()
		p.publishStatus()
		return nil
	})
	return err == nil && alive
}

func (p *Player) reportDropped(n int) {
	if n == 0 {
		return
	}
	p.cfg.Publish(TopicError, map[string]any{"message": fmt.Sprintf("left out %d tracks that couldn't be found", n), "dropped": n})
}
