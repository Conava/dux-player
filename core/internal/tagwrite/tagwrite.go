// Package tagwrite writes ratings and favorites into the tags of local files,
// for libraries that turned it on. Writes run one at a time in the
// background; the file that is playing is written once it stops being
// current.
package tagwrite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"go.senan.xyz/taglib"
	"golang.org/x/sys/unix"
)

const (
	Topic = "markFiles"

	// Rating is 0–100 in steps of 20 (Kodi, MusicBee, foobar2000's
	// 100-scale setting); FMPS_RATING is 0.0–1.0 (Strawberry, Clementine,
	// Amarok, Rhythmbox, Cantata). Love is MusicBee's favorite field, also
	// read by Picard and Symfonium: "L" is loved, "B" banned.
	keyRating = "RATING"
	keyFMPS   = "FMPS_RATING"
	keyLove   = "LOVE RATING"
	loved     = "L"

	maxProblems = 50
)

// Mark is a track's favorite flag and 0–5 star rating.
type Mark struct {
	Favorite bool `json:"favorite"`
	Rating   int  `json:"rating"`
}

// Fields says which marks of a Mark to write; the file's tags for the others
// stay as they are, so marks set by other players survive.
type Fields struct{ Favorite, Rating bool }

type Item struct {
	Path string
	Mark
	Fields Fields
}

// Files is what a library implements to have its marks written to its files.
type Files interface {
	TrackFile(ctx context.Context, id string) (Item, error)
	MarkedFiles(ctx context.Context) ([]Item, error)
}

// Codes of Problem and of a failed pass; the UI words them.
const (
	CodeNotRegularFile = "notRegularFile"
	CodeOtherOwner     = "otherOwner"
	CodeHardLinks      = "hardLinks"
	CodeReadOnlyFile   = "readOnlyFile"
	CodeReadOnlyFolder = "readOnlyFolder"
	CodeWriteFailed    = "writeFailed"
	CodeListFailed     = "listFailed"
	CodeBusy           = "busy"
)

const writeFailed = "Couldn't write the file"

// SkipError is a file that is left alone on purpose (read-only, another
// user's); other errors are failures.
type SkipError struct{ Code, Reason string }

func (e *SkipError) Error() string { return e.Reason }

func skip(code, reason string) error { return &SkipError{Code: code, Reason: reason} }

// wanted is the tags the fields f of m ask for, given the file's current
// tags: nil values remove a key. A ban set by another player is kept when
// unloving.
func wanted(old map[string][]string, m Mark, f Fields) map[string][]string {
	out := map[string][]string{}
	if f.Rating {
		out[keyRating], out[keyFMPS] = nil, nil
		if m.Rating > 0 {
			out[keyRating] = []string{strconv.Itoa(m.Rating * 20)}
			out[keyFMPS] = []string{strconv.FormatFloat(float64(m.Rating)/5, 'f', -1, 64)}
		}
	}
	if f.Favorite {
		switch {
		case m.Favorite:
			out[keyLove] = []string{loved}
		case len(old[keyLove]) == 0 || old[keyLove][0] == loved:
			out[keyLove] = nil
		}
	}
	for k, v := range out {
		if slices.Equal(old[k], v) || (len(old[k]) == 0 && len(v) == 0) {
			delete(out, k)
		}
	}
	return out
}

// Write puts the fields f of m into the file's tags: on a copy next to it,
// renamed over the file once written, keeping its other tags, its mode and
// the extended attributes (ACLs among them) the filesystem lets it copy. The
// mtime moves, so backup and sync tools see the change. It reports whether
// the file changed.
func Write(path string, m Mark, f Fields) (bool, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(real)
	if err != nil {
		return false, err
	}
	if err := writable(real, info); err != nil {
		return false, err
	}
	old, err := taglib.ReadTags(real)
	if err != nil {
		return false, fmt.Errorf("read the tags: %w", err)
	}
	tags := wanted(old, m, f)
	if len(tags) == 0 {
		return false, nil
	}
	dir, name := filepath.Split(real)
	sweepTemps(dir, name)
	// The copy keeps the extension: TagLib picks the format by it.
	tmp, err := os.CreateTemp(dir, "."+name+".*"+filepath.Ext(name))
	if err != nil {
		return false, err
	}
	done := false
	defer func() {
		if !done {
			os.Remove(tmp.Name())
		}
	}()
	if err := copyInto(tmp, real); err != nil {
		return false, err
	}
	if err := taglib.WriteTags(tmp.Name(), tags, 0); err != nil {
		return false, fmt.Errorf("write the tags: %w", err)
	}
	copyXattrs(tmp.Name(), real)
	if err := syncFile(tmp.Name(), info.Mode().Perm()); err != nil {
		return false, err
	}
	if now, err := os.Lstat(real); err != nil || now.Size() != info.Size() || !now.ModTime().Equal(info.ModTime()) {
		return false, errors.New("the file changed while its tags were written")
	}
	if err := os.Rename(tmp.Name(), real); err != nil {
		return false, err
	}
	done = true
	return true, nil
}

// sweepTemps removes copies of name that an earlier write left behind when
// the daemon stopped mid-write. Writes run one at a time, so none is in use.
func sweepTemps(dir, name string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix, ext := "."+name+".", filepath.Ext(name)
	for _, e := range entries {
		mid, ok := strings.CutPrefix(e.Name(), prefix)
		if !ok || !e.Type().IsRegular() {
			continue
		}
		// CreateTemp's random part is decimal digits.
		if mid, ok = strings.CutSuffix(mid, ext); ok && mid != "" && strings.Trim(mid, "0123456789") == "" {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func writable(path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return skip(CodeNotRegularFile, "Not a regular file")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(st.Uid) != os.Getuid() {
			return skip(CodeOtherOwner, "Owned by another user")
		}
		// Renaming over a hard-linked file would split it from its links.
		if st.Nlink > 1 {
			return skip(CodeHardLinks, "Has other hard links")
		}
	}
	if info.Mode().Perm()&0o200 == 0 {
		return skip(CodeReadOnlyFile, "Read-only file")
	}
	if syscall.Access(filepath.Dir(path), 0o3) != nil {
		return skip(CodeReadOnlyFolder, "Read-only folder")
	}
	return nil
}

func copyInto(dst *os.File, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(dst, in)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	return err
}

// copyXattrs gives dst the extended attributes of src; ones dst's
// filesystem or this user can't set are left out.
func copyXattrs(dst, src string) {
	buf := make([]byte, 64<<10)
	n, err := unix.Listxattr(src, buf)
	if err != nil || n <= 0 {
		return
	}
	val := make([]byte, 64<<10)
	for name := range strings.SplitSeq(strings.TrimRight(string(buf[:n]), "\x00"), "\x00") {
		if m, err := unix.Getxattr(src, name, val); err == nil {
			_ = unix.Setxattr(dst, name, val[:m], 0)
		}
	}
}

func syncFile(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Problem is a file that was skipped or failed. Reason is Code in plain
// English.
type Problem struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Skipped bool   `json:"skipped"`
}

// Job is a running "write existing marks" pass of one library.
type Job struct {
	Library string `json:"library"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Written int    `json:"written"`
	Failed  int    `json:"failed"`
}

// Last is the pass that ended last in this daemon's lifetime: State done
// with its counts, or failed with Code and Reason saying why it couldn't
// start.
type Last struct {
	Job
	State  string `json:"state"`
	Ended  int64  `json:"ended"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type Status struct {
	Pending int   `json:"pending"`
	Job     *Job  `json:"job"`
	Last    *Last `json:"last"`
	// Waiting is set while the playing file has a write waiting for it.
	Waiting  bool      `json:"waiting"`
	Problems []Problem `json:"problems"`
}

type entry struct {
	Item
	job bool
}

type Writer struct {
	publish func(topic string, data any)
	write   func(path string, m Mark, f Fields) (bool, error)

	mu       sync.Mutex
	queue    []entry
	current  string
	job      *Job
	last     *Last
	problems []Problem
	wake     chan struct{}
}

func New(publish func(topic string, data any)) *Writer {
	return &Writer{publish: publish, write: Write, wake: make(chan struct{}, 1)}
}

func (w *Writer) poke() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Enqueue queues a file's marks; a newer mark replaces a queued one and adds
// its fields to it.
func (w *Writer) Enqueue(it Item) {
	w.mu.Lock()
	w.add(entry{Item: it})
	w.mu.Unlock()
	w.poke()
}

func (w *Writer) add(e entry) {
	if i := slices.IndexFunc(w.queue, func(x entry) bool { return x.Path == e.Path }); i >= 0 {
		old := w.queue[i]
		if e.job && !old.job {
			// A pass lists its marks before it queues them: the mark queued by
			// hand is the newer one.
			e.Mark = old.Mark
		}
		e.job = e.job || old.job
		e.Fields.Favorite = e.Fields.Favorite || old.Fields.Favorite
		e.Fields.Rating = e.Fields.Rating || old.Fields.Rating
		w.queue[i] = e
		return
	}
	w.queue = append(w.queue, e)
}

// SetCurrent names the file that is playing; "" when none.
func (w *Writer) SetCurrent(path string) {
	w.mu.Lock()
	w.current = path
	w.mu.Unlock()
	w.poke()
}

// WriteAll queues the marks of every marked file of a library.
func (w *Writer) WriteAll(lib string, items []Item) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.job != nil {
		return fmt.Errorf("marks of %s are being written already", w.job.Library)
	}
	if len(items) == 0 {
		w.last = &Last{Job: Job{Library: lib}, State: "done", Ended: time.Now().Unix()}
		if w.publish != nil {
			go w.publish(Topic, map[string]any{"type": "done", "job": Job{Library: lib}})
		}
		return nil
	}
	w.job = &Job{Library: lib, Total: len(items)}
	for _, it := range items {
		w.add(entry{Item: it, job: true})
	}
	w.poke()
	return nil
}

func (w *Writer) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := Status{Pending: len(w.queue), Problems: slices.Clone(w.problems)}
	if st.Problems == nil {
		st.Problems = []Problem{}
	}
	if w.job != nil {
		j := *w.job
		st.Job = &j
	}
	if w.last != nil {
		l := *w.last
		st.Last = &l
	}
	st.Waiting = slices.ContainsFunc(w.queue, func(e entry) bool { return e.Path == w.current })
	return st
}

// next takes the first queued file that isn't playing.
func (w *Writer) next() (entry, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	i := slices.IndexFunc(w.queue, func(e entry) bool { return e.Path != w.current })
	if i < 0 {
		return entry{}, false
	}
	e := w.queue[i]
	w.queue = slices.Delete(w.queue, i, i+1)
	return e, true
}

// Run writes queued files one at a time until ctx ends.
func (w *Writer) Run(ctx context.Context) {
	for {
		e, ok := w.next()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			}
		}
		changed, err := w.write(e.Path, e.Mark, e.Fields)
		w.finish(e, changed, err)
		if ctx.Err() != nil {
			return
		}
	}
}

// Fail records a pass of lib that couldn't start and publishes it.
func (w *Writer) Fail(lib, code, reason string) {
	w.mu.Lock()
	w.last = &Last{Job: Job{Library: lib}, State: "failed", Ended: time.Now().Unix(), Code: code, Reason: reason}
	w.mu.Unlock()
	if w.publish != nil {
		w.publish(Topic, map[string]any{"type": "failed", "library": lib, "code": code, "reason": reason})
	}
}

func (w *Writer) finish(e entry, changed bool, err error) {
	w.mu.Lock()
	var pr Problem
	if err != nil {
		pr = Problem{Path: e.Path, Code: CodeWriteFailed, Reason: writeFailed}
		if se := (*SkipError)(nil); errors.As(err, &se) {
			pr.Code, pr.Reason, pr.Skipped = se.Code, se.Reason, true
		} else {
			log.Warnf("write marks to %s: %v", e.Path, err)
		}
		w.problems = append(w.problems, pr)
		if len(w.problems) > maxProblems {
			w.problems = w.problems[len(w.problems)-maxProblems:]
		}
	}
	var ev map[string]any
	if j := w.job; e.job && j != nil {
		j.Done++
		if err != nil {
			j.Failed++
		} else if changed {
			j.Written++
		}
		left := slices.ContainsFunc(w.queue, func(x entry) bool { return x.job })
		if !left {
			ev = map[string]any{"type": "done", "job": *j}
			w.last = &Last{Job: *j, State: "done", Ended: time.Now().Unix()}
			w.job = nil
		} else if j.Done%25 == 0 {
			ev = map[string]any{"type": "progress", "job": *j}
		}
	}
	if ev == nil && err != nil {
		ev = map[string]any{"type": "problem", "path": e.Path, "code": pr.Code, "reason": pr.Reason}
	}
	w.mu.Unlock()
	if ev != nil && w.publish != nil {
		w.publish(Topic, ev)
	}
}
