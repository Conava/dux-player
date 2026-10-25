package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"

	duxipc "github.com/duxlabs-dev/dux-go/ipc"
	"github.com/duxlabs-dev/dux-go/ipc/params"
	"github.com/duxlabs-dev/dux-go/log"
	"github.com/duxlabs-dev/dux-go/paths"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/settings"
	"github.com/Conava/dux-player/core/internal/window"
)

// Deps are the daemon's core services, shared by the features registered in
// this package. Handlers read them at call time, so registering with an
// empty Deps (as the CLI does) is fine.
type Deps struct {
	Bus       *Bus
	Player    *player.Player
	Libraries *library.Manager
	Settings  *settings.Store
	Window    *window.Launcher
	// FFmpeg says whether loudness measuring is possible.
	FFmpeg  bool
	Version string
	// Commit is the git commit the binary was built from ("unknown" without one).
	Commit string
	// Stats adds play counts to browse replies; nil without history.
	Stats interface{ Annotate(v any) }
	// CacheDir holds derived data such as cover colors.
	CacheDir string
	// Ctx outlives single requests; library scans run on it.
	Ctx  context.Context
	Quit func()
}

// RegisterCore adds the methods every client needs. ping, subscribe and
// unsubscribe are answered by Server itself; they are listed here only for
// the catalog. They are marked Read so they never wait behind a slow write.
func RegisterCore(r *Registry, d *Deps) {
	builtin := func(ctx context.Context, w *ConnWriter, req Request) {
		w.RespondError(req.ID, "unknown method: "+req.Method)
	}
	r.Group("core", builtin,
		MethodSpec{Name: "ping", Desc: "Liveness check", Read: true},
		MethodSpec{Name: "subscribe", Desc: "Subscribe to event topics", Params: []ParamSpec{Opt("topics", "comma-separated; default: every topic in the catalog")}, Read: true},
		MethodSpec{Name: "unsubscribe", Desc: "Unsubscribe from event topics", Params: []ParamSpec{Opt("topics", "topics to drop")}, Read: true},
	)
	r.Group("core", func(_ context.Context, w *ConnWriter, req Request) {
		switch req.Method {
		case "version":
			w.Respond(req.ID, map[string]any{"version": d.Version, "commit": d.Commit, "apiVersion": APIVersion})
		case "describe":
			w.Respond(req.ID, map[string]any{"apiVersion": APIVersion, "methods": r.Specs()})
		case "daemon.quit":
			w.Respond(req.ID, map[string]bool{"ok": true})
			d.Quit()
		}
	},
		MethodSpec{Name: "version", Desc: "Daemon version, its commit and the API version", Read: true},
		MethodSpec{Name: "describe", Desc: "List all IPC methods and their params", Read: true},
		MethodSpec{Name: "daemon.quit", Desc: "Stop the daemon and playback"},
	)
}

// Server is dux-go's socket protocol, except requests on one connection take
// effect in the order sent: dux-go runs each in its own goroutine, so
// queue.clear then queue.add could swap. Read methods run in parallel, each
// after the writes sent before it. Until Ready closes, only core methods run.
type Server struct {
	reg      *Registry
	bus      *Bus
	app      paths.App
	listener net.Listener
	socket   string
	closed   atomic.Bool
	Ready    <-chan struct{}
}

func NewServer(r *Registry, bus *Bus) *Server {
	return &Server{reg: r, bus: bus, app: paths.New(AppName)}
}

func (s *Server) Listen() error {
	removeStaleSockets(s.app)
	socket := s.app.SocketPath()
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	_ = os.Remove(socket)
	// Created 0600 rather than chmod'ed after: no moment where anyone may
	// connect. The umask is process-wide; owner rwx stays, so a directory
	// made at the same time still works.
	old := syscall.Umask(0o077)
	l, err := net.Listen("unix", socket)
	syscall.Umask(old)
	if err != nil {
		return fmt.Errorf("listen unix: %w", err)
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		l.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	s.listener, s.socket = l, socket
	log.Infof("ipc listening on %s", socket)
	return nil
}

func (s *Server) SocketPath() string { return s.socket }

func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return errors.New("listen has not been called")
	}
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	for {
		c, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go s.serveConn(ctx, c)
	}
}

func (s *Server) Close() error {
	if s.closed.Swap(true) || s.listener == nil {
		return nil
	}
	_ = os.Remove(s.socket)
	return s.listener.Close()
}

func (s *Server) serveConn(ctx context.Context, c net.Conn) {
	defer c.Close()
	caps := duxipc.Capabilities{APIVersion: APIVersion, Capabilities: append(s.reg.groups(), "subscribe")}
	if err := json.NewEncoder(c).Encode(caps); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	q := newQueuedConn(c)
	defer q.Close()
	w := duxipc.NewConnWriter(q)
	sub := s.bus.NewSubscriber(ctx, w)
	defer func() {
		sub.Close()
		s.subscriptionsChanged(ctx)
	}()

	// lastWrite closes when the latest write has been answered.
	lastWrite := make(chan struct{})
	close(lastWrite)
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64<<10), maxRequestLine)
	for sc.Scan() {
		var req Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			w.RespondError(0, "invalid json: "+err.Error())
			continue
		}
		after := lastWrite
		if s.reg.isRead(req.Method) {
			go func() {
				<-after
				s.dispatch(ctx, q, w, req, sub)
			}()
			continue
		}
		done := make(chan struct{})
		lastWrite = done
		go func() {
			defer close(done)
			<-after
			s.dispatch(ctx, q, w, req, sub)
		}()
	}
}

const maxRequestLine = 1 << 20

func (s *Server) dispatch(ctx context.Context, q *queuedConn, w *ConnWriter, req Request, sub *Subscriber) {
	rc := &replyConn{Conn: q}
	s.handle(ctx, duxipc.NewConnWriter(rc), req, sub)
	if !rc.wrote.Load() {
		w.RespondError(req.ID, req.Method+": the reply couldn't be encoded")
	}
}

func (s *Server) handle(ctx context.Context, w *ConnWriter, req Request, sub *Subscriber) {
	switch req.Method {
	case "ping":
		w.Respond(req.ID, map[string]any{"pong": true})
	case "subscribe":
		topics := params.StringSlice(req.Params, "topics")
		if len(topics) == 0 {
			topics = s.reg.topics
		}
		sub.Subscribe(topics...)
		w.Respond(req.ID, map[string]any{"topics": sub.Topics()})
		if !s.subscriptionsChanged(ctx) {
			return
		}
		for _, f := range s.reg.greet {
			f(func(topic string, data any) {
				if slices.Contains(topics, topic) {
					_ = w.WriteEvent(duxipc.Event{Topic: topic, Data: data})
				}
			})
		}
	case "unsubscribe":
		sub.Unsubscribe(params.StringSlice(req.Params, "topics")...)
		w.Respond(req.ID, map[string]any{"topics": sub.Topics()})
		s.subscriptionsChanged(ctx)
	default:
		if m, ok := s.reg.Find(req.Method); (!ok || m.Group != "core") && !s.ready(ctx) {
			return
		}
		s.reg.route(ctx, w, req)
	}
}

// subscriptionsChanged runs the OnSubscribe hooks once startup is done; false
// when ctx ended first.
func (s *Server) subscriptionsChanged(ctx context.Context) bool {
	if !s.ready(ctx) {
		return false
	}
	for _, f := range s.reg.onSubscribe {
		f()
	}
	return true
}

func (s *Server) ready(ctx context.Context) bool {
	if s.Ready == nil {
		return true
	}
	select {
	case <-s.Ready:
		return true
	case <-ctx.Done():
		return false
	}
}

// removeStaleSockets deletes sockets of daemons that are gone, so
// FindRunningSocket doesn't dial dead ones.
func removeStaleSockets(app paths.App) {
	dir := app.SocketDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, ok := strings.CutPrefix(e.Name(), app.Name+"-")
		if !ok {
			continue
		}
		pid, ok = strings.CutSuffix(pid, ".sock")
		n, err := strconv.Atoi(pid)
		if !ok || err != nil {
			continue
		}
		if syscall.Kill(n, 0) == nil {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}
