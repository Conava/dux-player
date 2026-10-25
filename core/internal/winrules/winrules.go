// Package winrules implements "Open floating", a compositor window rule for
// the player's windows: through `dms config windowrules` with Dux Shell, else
// as a snippet the user adds by hand.
package winrules

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

const (
	Topic    = "winrules"
	RuleID   = "dplayer-floating-windows"
	fileName = "winrules.json"
	timeout  = 5 * time.Second
	maxOut   = 1 << 20

	ModeShell       = "shell"
	ModeManual      = "manual"
	ModeUnsupported = "unsupported"
)

// The settings window is a child of the main one with the same app id, so
// one rule covers both.
var rule = `{"id":"` + RuleID + `","enabled":true,"name":"Dux Player Floating Windows","matchCriteria":{"appId":"^(com\\.marlonkranz\\.player)$"},"actions":{"openFloating":true}}`

type Status struct {
	// Compositor is hyprland, niri, mango or "".
	Compositor string `json:"compositor"`
	// Mode is shell (a toggle), manual (Snippet to add by hand) or unsupported.
	Mode    string `json:"mode"`
	Active  bool   `json:"active"`
	Snippet string `json:"snippet,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Detect names the running compositor from its environment.
func Detect(getenv func(string) string) string {
	switch {
	case getenv("HYPRLAND_INSTANCE_SIGNATURE") != "":
		return "hyprland"
	case getenv("NIRI_SOCKET") != "":
		return "niri"
	case getenv("MANGO_INSTANCE_SIGNATURE") != "" || strings.Contains(strings.ToLower(getenv("XDG_CURRENT_DESKTOP")), "mango"):
		return "mango"
	}
	return ""
}

func listArgv(comp string) []string { return []string{"dms", "config", "windowrules", "list", comp} }
func addArgv(comp string) []string {
	return []string{"dms", "config", "windowrules", "add", comp, rule}
}
func removeArgv(comp string) []string {
	return []string{"dms", "config", "windowrules", "remove", comp, RuleID}
}

// Snippet is the rule to add by hand when the shell isn't there.
func Snippet(comp string) string {
	switch comp {
	case "hyprland":
		return "-- hyprland.lua\nhl.window_rule({ match = { class = \"^(com\\\\.marlonkranz\\\\.player)$\" }, float = true })\n\n" +
			"# hyprland.conf\nwindowrule = float, class:^(com\\.marlonkranz\\.player)$"
	case "niri":
		return "window-rule {\n    match app-id=r#\"^com\\.marlonkranz\\.player$\"#\n    open-floating true\n}"
	case "mango":
		return "windowrule=isfloating:1,appid:^com\\.marlonkranz\\.player$"
	}
	return ""
}

// active reads `dms config windowrules list` output.
func active(out string) (bool, error) {
	var list struct {
		Rules []struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
			Actions struct {
				OpenFloating *bool `json:"openFloating"`
			} `json:"actions"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return false, fmt.Errorf("read window rules: %w", err)
	}
	for _, r := range list.Rules {
		if r.ID == RuleID && (r.Enabled == nil || *r.Enabled) && r.Actions.OpenFloating != nil && *r.Actions.OpenFloating {
			return true, nil
		}
	}
	return false, nil
}

type Service struct {
	comp  string
	shell bool
	path  string
	// run and reload are replaced in tests.
	run    func(ctx context.Context, argv []string) (string, error)
	reload func(ctx context.Context, comp string) error

	mu     sync.Mutex
	status Status
	seeded []string
	// sets counts Set calls, so a Refresh that overlapped one keeps its result.
	sets int
}

func New(configDir string) *Service {
	_, err := exec.LookPath("dms")
	// DPLAYER_WINRULES=off keeps test and sandbox runs from writing rules or reloading the live compositor.
	shell := err == nil && os.Getenv("DPLAYER_WINRULES") != "off"
	s := &Service{comp: Detect(os.Getenv), shell: shell, path: filepath.Join(configDir, fileName), run: runCmd, reload: reload}
	s.status = Status{Compositor: s.comp, Mode: ModeUnsupported}
	switch {
	case s.comp != "" && s.shell:
		s.status.Mode = ModeShell
	case s.comp != "":
		s.status.Mode, s.status.Snippet = ModeManual, Snippet(s.comp)
	}
	if err := s.load(); err != nil {
		log.Warnf("winrules: %v", err)
	}
	return s
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Seed turns the rule on the first time the player runs under a compositor
// (it's on by default, like the shell's own). Later starts run nothing:
// winrules.status reads the state when the UI asks.
func (s *Service) Seed(ctx context.Context) (Status, error) {
	if s.Status().Mode != ModeShell {
		return s.Status(), nil
	}
	s.mu.Lock()
	seeded := slices.Contains(s.seeded, s.comp)
	s.mu.Unlock()
	if seeded {
		return s.Status(), nil
	}
	st, err := s.Set(ctx, true)
	if err != nil {
		return st, err
	}
	s.mu.Lock()
	s.seeded = append(s.seeded, s.comp)
	s.mu.Unlock()
	return st, s.save()
}

// Refresh reads whether the rule is in place.
func (s *Service) Refresh(ctx context.Context) (Status, error) {
	if s.Status().Mode != ModeShell {
		return s.Status(), nil
	}
	s.mu.Lock()
	sets := s.sets
	s.mu.Unlock()
	out, err := s.run(ctx, listArgv(s.comp))
	var on bool
	if err == nil {
		on, err = active(out)
	}
	s.mu.Lock()
	stale := s.sets != sets
	s.mu.Unlock()
	if stale {
		return s.Status(), nil
	}
	return s.finish(on, err)
}

// Set adds or removes the rule and has the compositor reload it.
func (s *Service) Set(ctx context.Context, enabled bool) (Status, error) {
	if s.Status().Mode != ModeShell {
		return s.Status(), errors.New("open floating needs Dux Shell; add the rule by hand")
	}
	s.mu.Lock()
	s.sets++
	s.mu.Unlock()
	argv := removeArgv(s.comp)
	if enabled {
		argv = addArgv(s.comp)
	}
	if _, err := s.run(ctx, argv); err != nil {
		return s.finish(!enabled, err)
	}
	if err := s.reload(ctx, s.comp); err != nil {
		log.Warnf("winrules: reload %s: %v", s.comp, err)
	}
	return s.finish(enabled, nil)
}

func (s *Service) finish(on bool, err error) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Active, s.status.Error = on, ""
	if err != nil {
		s.status.Error = err.Error()
	}
	return s.status, err
}

func runCmd(ctx context.Context, argv []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = capped{&out}, capped{&errOut}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %v: %s", strings.Join(argv[:min(len(argv), 4)], " "), err, strings.TrimSpace(out.String()+errOut.String()))
	}
	return out.String(), nil
}

// reload makes the compositor pick up the rule file, as the shell does
// after a rule write. niri watches its config by itself.
func reload(ctx context.Context, comp string) error {
	switch comp {
	case "hyprland":
		_, err := runCmd(ctx, []string{"hyprctl", "reload"})
		return err
	case "mango":
		return mangoReload(os.Getenv("MANGO_INSTANCE_SIGNATURE"))
	}
	return nil
}

// mangoReload sends one dispatch over mango's IPC socket, which answers
// with one JSON line and closes.
func mangoReload(socket string) error {
	if socket == "" {
		return errors.New("MANGO_INSTANCE_SIGNATURE is not set")
	}
	c, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	if _, err := c.Write([]byte("dispatch reload_config\n")); err != nil {
		return err
	}
	line, err := bufio.NewReader(io.LimitReader(c, maxOut)).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("no reply from mango: %w", err)
	}
	var reply struct {
		Success bool `json:"success"`
	}
	if json.Unmarshal([]byte(line), &reply) != nil || !reply.Success {
		return fmt.Errorf("mango refused reload_config: %s", strings.TrimSpace(line))
	}
	return nil
}

// capped keeps the first maxOut bytes of a command's output.
type capped struct{ *bytes.Buffer }

func (c capped) Write(p []byte) (int, error) {
	if room := maxOut - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

type saved struct {
	Seeded []string `json:"floatingSeeded"`
}

func (s *Service) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var v saved
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	s.seeded = v.Seeded
	return nil
}

func (s *Service) save() error {
	s.mu.Lock()
	data, err := json.Marshal(saved{Seeded: s.seeded})
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(s.path, append(data, '\n'), 0o600)
}
