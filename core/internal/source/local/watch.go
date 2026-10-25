package local

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

// Changes are applied once the folder has been quiet for debounce, or at
// the latest maxDelay after the first change (a long copy). Variables so
// tests can shorten them.
var (
	debounce = 2 * time.Second
	maxDelay = 30 * time.Second
)

// More changed paths than this in one burst turn into a full rescan.
const maxPending = 4096

// watch keeps the index current while the library is open: changed files
// are reread, deleted ones dropped, vanished folders marked missing, and a
// drive coming or going is noticed through mount changes.
func (s *Source) watch() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warnf("watch %s: %v", s.lib.Name, err)
		s.watchNotice.Store(source.Notice{Code: source.NoticeWatchFailed, Message: "Changes on disk show only after a rescan"})
		return
	}
	defer w.Close()
	dirs := map[string]bool{}
	for _, root := range Folders(s.lib) {
		s.addWatches(w, dirs, root)
	}
	mounts := mountChanges(s.ctx)

	pending := map[string]bool{}
	full := false
	var first time.Time
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	touch := func() {
		if first.IsZero() {
			first = time.Now()
		}
		timer.Reset(min(debounce, maxDelay-time.Since(first)))
	}
	for {
		select {
		case <-s.ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Op == fsnotify.Chmod {
				continue
			}
			if ev.Has(fsnotify.Create) {
				if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
					s.addWatches(w, dirs, ev.Name)
				}
			}
			if len(pending) < maxPending {
				pending[ev.Name] = true
			} else {
				full = true
			}
			touch()
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Warnf("library %s: watcher: %v", s.lib.Name, err)
			full = full || errors.Is(err, fsnotify.ErrEventOverflow)
			touch()
		case <-mounts:
			touch()
		case <-timer.C:
			s.flush(w, dirs, pending, full)
			pending, full, first = map[string]bool{}, false, time.Time{}
		}
	}
}

// addWatches watches dir and every folder below it that a scan would read.
// Running out of inotify watches leaves a notice instead of failing.
func (s *Source) addWatches(w *fsnotify.Watcher, dirs map[string]bool, dir string) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		if p != dir && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if dirs[p] {
			return nil
		}
		if err := w.Add(p); err != nil {
			if errors.Is(err, syscall.ENOSPC) {
				s.watchNotice.Store(source.Notice{Code: source.NoticeTooManyFolders,
					Message: "Too many folders to watch (fs.inotify.max_user_watches); some changes show only after a rescan"})
				return filepath.SkipAll
			}
			return nil
		}
		dirs[p] = true
		return nil
	})
}

// flush applies a burst of changes. A path that vanished while its folder
// is still there was deleted; a vanished folder is marked missing, since it
// may come back (an unmounted drive, a network share).
func (s *Source) flush(w *fsnotify.Watcher, dirs map[string]bool, pending map[string]bool, full bool) {
	ctx := s.ctx
	changed, back := s.checkRoots(ctx)
	for _, root := range back {
		s.addWatches(w, dirs, root)
	}
	if full || len(back) > 0 {
		if _, err := s.Scan(ctx, nil); err != nil {
			log.Warnf("library %s: rescan after changes: %v", s.lib.Name, err)
		}
		s.store.Changed(s.lib.ID, nil)
		return
	}
	roots := Folders(s.lib)
	seen := map[string]bool{}
	var files, scope []string
	missing := map[string]bool{}
	addFile := func(p string) {
		if !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	for p := range pending {
		root := rootOf(roots, p)
		if root == "" || s.isGone(root) || hidden(root, p) {
			continue
		}
		info, err := os.Lstat(p)
		switch {
		case err == nil && info.IsDir():
			found, _ := Collect(ctx, p)
			for _, f := range found {
				addFile(f)
			}
			scope = append(scope, p)
		case err == nil && info.Mode().IsRegular() && IsAudio(p):
			addFile(p)
			scope = append(scope, p)
		case errors.Is(err, fs.ErrNotExist):
			if g := vanishedFolder(root, p); g != "" || dirs[p] {
				if g == "" {
					g = p
				}
				missing[g] = true
			} else {
				scope = append(scope, p)
			}
		}
	}
	// Folders that went or came back aren't in the delta: then listeners
	// reload instead.
	var d *library.Delta
	if !changed {
		d = &library.Delta{}
	}
	if len(scope) > 0 {
		stats, err := s.index(ctx, files, nil, scope, nil, d)
		if err != nil {
			log.Warnf("library %s: apply changes: %v", s.lib.Name, err)
		}
		changed = changed || stats.Updated > 0 || stats.Removed > 0
	}
	for dir := range missing {
		forgetDirs(dirs, dir)
		if ok, err := s.store.MarkMissing(ctx, s.lib.ID, dir); err == nil && ok {
			changed, d = true, nil
		}
	}
	if changed {
		s.store.Changed(s.lib.ID, d)
	}
}

// checkRoots updates which library folders are gone, marks the tracks of
// newly gone ones missing, and returns the folders that came back.
func (s *Source) checkRoots(ctx context.Context) (changed bool, back []string) {
	gone := map[string]bool{}
	for _, root := range Folders(s.lib) {
		if !s.looksGone(ctx, root) {
			if s.isGone(root) {
				back = append(back, root)
			}
			continue
		}
		gone[root] = true
		if ok, _ := s.store.MarkMissing(ctx, s.lib.ID, root); ok {
			changed = true
		}
	}
	old := s.goneRoots()
	s.gone.Store(&gone)
	return changed || len(old) != len(gone), back
}

func (s *Source) goneRoots() map[string]bool {
	if g := s.gone.Load(); g != nil {
		return *g
	}
	return nil
}

func (s *Source) isGone(root string) bool { return s.goneRoots()[root] }

// rootOf returns the library folder p lies in, or "".
func rootOf(roots []string, p string) string {
	for _, r := range roots {
		if p == r || strings.HasPrefix(p, r+string(filepath.Separator)) {
			return r
		}
	}
	return ""
}

func hidden(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}
	return false
}

// vanishedFolder returns the topmost folder between root and p (exclusive)
// that no longer exists, or "" when p's own folder is still there.
func vanishedFolder(root, p string) string {
	g := ""
	for dir := filepath.Dir(p); dir != root && len(dir) > len(root); dir = filepath.Dir(dir) {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		g = dir
	}
	return g
}

func forgetDirs(dirs map[string]bool, dir string) {
	for d := range dirs {
		if d == dir || strings.HasPrefix(d, dir+string(filepath.Separator)) {
			delete(dirs, d)
		}
	}
}

// mountChanges signals whenever a filesystem is mounted or unmounted. The
// kernel flags /proc/self/mountinfo with POLLPRI on every change, so this
// blocks instead of polling. Without it, drives are noticed on rescans only.
func mountChanges(ctx context.Context) <-chan struct{} {
	out := make(chan struct{}, 1)
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return out
	}
	var stop [2]int
	if err := unix.Pipe2(stop[:], unix.O_CLOEXEC); err != nil {
		f.Close()
		return out
	}
	go func() {
		<-ctx.Done()
		unix.Close(stop[1])
	}()
	go func() {
		defer f.Close()
		defer unix.Close(stop[0])
		fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLPRI}, {Fd: int32(stop[0]), Events: unix.POLLIN}}
		for {
			fds[0].Revents, fds[1].Revents = 0, 0
			if _, err := unix.Poll(fds, -1); err != nil {
				if errors.Is(err, unix.EINTR) {
					continue
				}
				return
			}
			if fds[1].Revents != 0 || ctx.Err() != nil {
				return
			}
			if fds[0].Revents&(unix.POLLPRI|unix.POLLERR) != 0 {
				// Reading it again re-arms the notification.
				drain(f)
				select {
				case out <- struct{}{}:
				default:
				}
			}
		}
	}()
	return out
}

func drain(f *os.File) {
	_, _ = f.Seek(0, 0)
	buf := make([]byte, 64<<10)
	for {
		if _, err := f.Read(buf); err != nil {
			return
		}
	}
}
