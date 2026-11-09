// Package discord shows the playing track on the user's Discord profile through Discord's local IPC socket.
package discord

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Conava/dux-player/core/internal/source"
	"github.com/duxlabs-dev/dux-go/log"
)

// AppID is the Dux Player application in the Discord developer portal; LargeImageKey names its logo art asset.
var AppID, LargeImageKey = "1569104598336278579", "logo"

const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4

	maxFrame    = 64 << 10
	maxFieldLen = 128 // Discord's limit for each text field
)

// Vars so tests can shorten them.
var (
	backoffMin  = 5 * time.Second
	backoffMax  = 5 * time.Minute
	minInterval = 4 * time.Second // Discord allows about 5 SET_ACTIVITY per 20 s
	ioTimeout   = time.Second
)

var errRejected = errors.New("handshake rejected: the application id is unknown to Discord")

// Activity is the track shown on the card.
type Activity struct {
	Title, Artist, Album string
	Start                time.Time
}

// Client keeps Discord's card in line with the newest wanted activity. Its methods never block on Discord.
type Client struct {
	mu      sync.Mutex
	want    *Activity
	started bool
	closed  bool
	wake    chan struct{}
	done    chan struct{}
	stopped chan struct{}
}

// New returns a client that does nothing until the first Set.
func New() *Client {
	return &Client{wake: make(chan struct{}, 1), done: make(chan struct{}), stopped: make(chan struct{})}
}

// Set shows a on the card, starting the connection on first use.
func (c *Client) Set(a Activity) { c.update(&a) }

// Clear removes the card.
func (c *Client) Clear() { c.update(nil) }

func (c *Client) update(a *Activity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.want = a
	if !c.started {
		if a == nil {
			return
		}
		c.started = true
		go c.run()
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Close clears the card, closes the socket and stops the goroutine.
func (c *Client) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	started := c.started
	c.mu.Unlock()
	close(c.done)
	if started {
		<-c.stopped
	}
}

func (c *Client) run() {
	defer close(c.stopped)
	var s session
	defer s.shutdown()
	backoff := backoffMin
	logged := ""
	for {
		c.mu.Lock()
		want := c.want
		c.mu.Unlock()
		if !s.differs(want) {
			select {
			case <-c.done:
				return
			case <-c.wake:
				continue
			}
		}
		if wait := minInterval - time.Since(s.lastSent); wait > 0 {
			if !c.sleep(wait) {
				return
			}
			continue
		}
		err := s.send(want)
		if err == nil {
			backoff, logged = backoffMin, ""
			continue
		}
		delay := backoff
		backoff = min(backoff*2, backoffMax)
		if errors.Is(err, errRejected) {
			delay, backoff = backoffMax, backoffMax
		}
		if err.Error() != logged {
			logged = err.Error()
			log.Infof("discord presence: %v", err)
		}
		if !c.sleep(delay) {
			return
		}
	}
}

// sleep waits d unless Close comes first. Set doesn't cut it short, so backoff and rate limit hold.
func (c *Client) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.done:
		return false
	case <-t.C:
		return true
	}
}

// session is the connection and what Discord currently shows from us. Only run touches it.
type session struct {
	conn     net.Conn
	known    bool // sent holds what Discord shows; false after a drop, when Discord has cleared it itself
	sent     *Activity
	lastSent time.Time
	nonce    int
}

func (s *session) differs(want *Activity) bool {
	if !s.known {
		return want != nil
	}
	if want == nil || s.sent == nil {
		return want != s.sent
	}
	return want.Title != s.sent.Title || want.Artist != s.sent.Artist || want.Album != s.sent.Album ||
		!want.Start.Equal(s.sent.Start)
}

func (s *session) send(want *Activity) error {
	fresh := s.conn == nil
	if fresh {
		if err := s.connect(); err != nil {
			return err
		}
	}
	err := s.setActivity(want)
	if err != nil && !fresh {
		// An idle connection may have died unseen; one fresh try before backing off.
		s.drop()
		if err = s.connect(); err != nil {
			return err
		}
		err = s.setActivity(want)
	}
	if err != nil {
		s.drop()
		return err
	}
	s.sent, s.known, s.lastSent = want, true, time.Now()
	return nil
}

func (s *session) drop() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.conn, s.known, s.sent = nil, false, nil
}

func (s *session) shutdown() {
	if s.conn == nil {
		return
	}
	if s.known && s.sent != nil {
		_ = s.setActivity(nil)
	}
	s.drop()
}

func (s *session) connect() error {
	conn, err := dial()
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(ioTimeout))
	if err := writeFrame(conn, opHandshake, map[string]any{"v": 1, "client_id": AppID}); err != nil {
		_ = conn.Close()
		return fmt.Errorf("handshake: %w", err)
	}
	if err := readReady(conn); err != nil {
		_ = conn.Close()
		return err
	}
	s.conn = conn
	return nil
}

func dial() (net.Conn, error) {
	var dirs []string
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		dirs = append(dirs, run, filepath.Join(run, "app", "com.discordapp.Discord"), filepath.Join(run, "snap.discord"))
	}
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	dirs = append(dirs, tmp)
	for _, dir := range dirs {
		for i := range 10 {
			conn, err := net.DialTimeout("unix", filepath.Join(dir, "discord-ipc-"+strconv.Itoa(i)), ioTimeout)
			if err == nil {
				return conn, nil
			}
		}
	}
	return nil, errors.New("no Discord socket found")
}

type reply struct {
	Cmd     string `json:"cmd"`
	Evt     string `json:"evt"`
	Nonce   string `json:"nonce"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"data"`
}

// readReady takes the handshake answer. A close or an error event means Discord doesn't know AppID.
func readReady(conn net.Conn) error {
	for {
		op, r, err := readReply(conn)
		switch {
		case err != nil:
			return fmt.Errorf("handshake: %w", err)
		case op == opClose:
			return fmt.Errorf("%w (%d %s)", errRejected, r.Code, source.Clean(r.Message))
		case op == opFrame && r.Evt == "ERROR":
			return fmt.Errorf("%w (%d %s)", errRejected, r.Data.Code, source.Clean(r.Data.Message))
		case op == opFrame && r.Evt == "READY":
			return nil
		}
	}
}

func (s *session) setActivity(a *Activity) error {
	s.nonce++
	nonce := strconv.Itoa(s.nonce)
	_ = s.conn.SetDeadline(time.Now().Add(ioTimeout))
	cmd := map[string]any{
		"cmd":   "SET_ACTIVITY",
		"args":  map[string]any{"pid": os.Getpid(), "activity": activityJSON(a)},
		"nonce": nonce,
	}
	if err := writeFrame(s.conn, opFrame, cmd); err != nil {
		return err
	}
	for {
		op, r, err := readReply(s.conn)
		switch {
		case err != nil:
			return err
		case op == opClose:
			return fmt.Errorf("connection closed by Discord (%d %s)", r.Code, source.Clean(r.Message))
		case op == opFrame && r.Nonce == nonce:
			if r.Evt == "ERROR" {
				// Keep it as sent: resending the same activity would fail the same way.
				log.Debugf("discord presence: activity refused: %s", source.Clean(r.Data.Message))
			}
			return nil
		}
	}
}

// readReply reads one frame and answers a ping on the way.
func readReply(conn net.Conn) (uint32, reply, error) {
	var r reply
	op, body, err := readFrame(conn)
	if err != nil {
		return 0, r, err
	}
	if op == opPing {
		return op, r, writeFrame(conn, opPong, json.RawMessage(body))
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, r, fmt.Errorf("bad reply from Discord: %w", err)
	}
	return op, r, nil
}

// activityJSON builds the card. Never the cover: its URL carries the server token.
func activityJSON(a *Activity) any {
	if a == nil {
		return nil
	}
	out := map[string]any{"type": 2} // 2: Listening
	if details := field(a.Title); details != "" {
		if utf8.RuneCountInString(details) == 1 {
			// Discord refuses text shorter than 2 characters and trims spaces, so pad with a zero width space.
			details += "\u200b"
		}
		out["details"] = details
	}
	if artist := field(a.Artist); artist != "" {
		out["state"] = field("by " + artist)
	}
	assets := map[string]string{"large_image": LargeImageKey}
	if album := field(a.Album); utf8.RuneCountInString(album) >= 2 {
		assets["large_text"] = album
	}
	out["assets"] = assets
	if !a.Start.IsZero() {
		out["timestamps"] = map[string]int64{"start": a.Start.Unix()}
	}
	return out
}

func field(s string) string {
	r := []rune(source.Clean(s))
	if len(r) > maxFieldLen {
		r = r[:maxFieldLen]
	}
	return string(r)
}

func writeFrame(w io.Writer, op uint32, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	buf := make([]byte, 8, 8+len(data))
	binary.LittleEndian.PutUint32(buf, op)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(data)))
	_, err = w.Write(append(buf, data...))
	return err
}

func readFrame(r io.Reader) (uint32, []byte, error) {
	var head [8]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, closedErr(err)
	}
	n := binary.LittleEndian.Uint32(head[4:])
	if n > maxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes from Discord is too large", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, closedErr(err)
	}
	return binary.LittleEndian.Uint32(head[:4]), body, nil
}

func closedErr(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("connection closed by Discord")
	}
	return err
}
