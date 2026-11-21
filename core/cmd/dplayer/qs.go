package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/duxlabs-dev/dux-go/qtbase"
	"github.com/ebitengine/purego"
)

// runtimeChoice is where the UI runs: the dux-qt-base library at lib, or the
// qs binary at qs. reason says why the library was passed over; it is empty
// when qs was asked for.
type runtimeChoice struct {
	lib, qs, reason string
}

var errNoRuntime = errors.New("the UI needs dux-qt-base (libdux-qt-base.so.0) or Quickshell (qs); see Requirements in the README")

// pickRuntime prefers dux-qt-base and falls back to qs. forceQS skips the
// library without loading it.
func pickRuntime(forceQS bool, load func() (string, error), lookPath func(string) (string, error)) (runtimeChoice, error) {
	var reason string
	if !forceQS {
		lib, err := load()
		if err == nil {
			return runtimeChoice{lib: lib}, nil
		}
		reason = err.Error()
	}
	path, err := lookPath(qtbase.NativeBinary)
	if err != nil {
		return runtimeChoice{}, errNoRuntime
	}
	return runtimeChoice{qs: path, reason: reason}, nil
}

// loadQtBase loads the library into this process, which catches one built
// against another Qt before the UI has started.
func loadQtBase() (string, error) {
	lib, err := qtbase.Locate(nil)
	if err != nil {
		return "", err
	}
	if _, err := purego.Dlopen(lib, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return "", err
	}
	return lib, nil
}

// runUIIfAsked turns this process into the UI when started as `dplayer qs`:
// dux-qt-base when it loads, else the qs binary. The window launcher starts
// every UI this way. It returns for any other command.
func runUIIfAsked() {
	if len(os.Args) < 2 || os.Args[1] != qtbase.Subcommand {
		// A crash handler relaunches the process without arguments.
		qtbase.MaybeRunAndExit(nil)
		return
	}
	choice, err := pickRuntime(os.Getenv("DPLAYER_QS_CHILD") != "", loadQtBase, exec.LookPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(127)
	}
	if choice.lib != "" {
		qtbase.MaybeRunAndExit(func() (string, error) { return choice.lib, nil })
		return
	}
	if choice.reason != "" {
		fmt.Fprintf(os.Stderr, "dplayer: dux-qt-base unavailable (%s); running the UI on Quickshell (qs)\n", choice.reason)
	}
	err = syscall.Exec(choice.qs, append([]string{qtbase.NativeBinary}, os.Args[2:]...), os.Environ())
	fmt.Fprintf(os.Stderr, "dplayer: start qs: %v\n", err)
	os.Exit(127)
}
