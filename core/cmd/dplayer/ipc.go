package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/Conava/dux-player/core/internal/ipc"
)

var ipcCmd = &cobra.Command{
	Use:   "ipc <method> [key=value...]",
	Short: "Send an IPC request to the dplayer daemon, starting it if needed",
	Long: `Send an IPC request to the dplayer daemon, starting it if needed.

  dplayer ipc <method> [key=value...]   invoke a method
  dplayer ipc list                      list every method and its params

A key given twice, or a value written as a JSON array, is sent as a list:
  dplayer ipc player.play paths=/music/a paths=/music/b

password=- reads the password from stdin (typed without echo on a terminal).

Method names and param keys complete with the shell (see 'dplayer completion').`,
	Args:                  cobra.MinimumNArgs(1),
	ValidArgsFunction:     completeIPC,
	DisableFlagsInUseLine: true,
	RunE: func(_ *cobra.Command, args []string) error {
		method := args[0]
		params, err := parseParams(args[1:])
		if err != nil {
			return err
		}
		switch params["password"] {
		case nil:
		case "-":
			if params["password"], err = readSecret("password"); err != nil {
				return err
			}
		default:
			fmt.Fprintln(os.Stderr, "warning: a password on the command line is visible to other processes; use password=- to type it")
		}

		var result any
		if err := call(true, method, params, &result); err != nil {
			return err
		}

		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}

var ipcListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every IPC method and its params",
	RunE: func(_ *cobra.Command, _ []string) error {
		if jsonOutput {
			return printJSON(map[string]any{"apiVersion": ipc.APIVersion, "methods": newRegistry(&daemon{}).Specs()})
		}
		printIPCList()
		return nil
	},
}

func init() {
	ipcCmd.AddCommand(ipcListCmd)
}

func completeIPC(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return newRegistry(&daemon{}).Names(), cobra.ShellCompDirectiveNoFileComp
	}

	spec, ok := newRegistry(&daemon{}).Find(args[0])
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	used := make(map[string]bool, len(args))
	for _, a := range args[1:] {
		if k, _, found := strings.Cut(a, "="); found {
			used[k] = true
		}
	}

	comps := make([]string, 0, len(spec.Params))
	for _, p := range spec.Params {
		if used[p.Name] {
			continue
		}
		desc := p.Desc
		if p.Required {
			desc = strings.TrimSpace("required " + desc)
		}
		comps = append(comps, fmt.Sprintf("%s=\t%s", p.Name, desc))
	}
	return comps, cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

func printIPCList() {
	byGroup := make(map[string][]ipc.MethodSpec)
	groups := make([]string, 0)
	for _, m := range newRegistry(&daemon{}).Specs() {
		if _, seen := byGroup[m.Group]; !seen {
			groups = append(groups, m.Group)
		}
		byGroup[m.Group] = append(byGroup[m.Group], m)
	}
	sort.Strings(groups)

	for _, group := range groups {
		fmt.Printf("%s\n", group)
		for _, m := range byGroup[group] {
			fmt.Printf("  %-32s %s\n", m.Name, m.Desc)
			for _, p := range m.Params {
				marker := " "
				if p.Required {
					marker = "*"
				}
				line := fmt.Sprintf("      %s%s", marker, p.Name)
				if p.Desc != "" {
					line = fmt.Sprintf("%-32s %s", line, p.Desc)
				}
				fmt.Println(line)
			}
		}
		fmt.Println()
	}
	fmt.Println("* = required   |   usage: dplayer ipc <method> key=value ...")
}

func parseParams(args []string) (map[string]any, error) {
	if len(args) == 0 {
		return nil, nil
	}
	params := make(map[string]any, len(args))
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value, got %q", arg)
		}
		var list []any
		isList := strings.HasPrefix(value, "[") && json.Unmarshal([]byte(value), &list) == nil
		prev, seen := params[key]
		switch {
		case !seen && isList:
			params[key] = list
		case !seen:
			params[key] = value
		case isList:
			params[key] = append(asList(prev), list...)
		default:
			params[key] = append(asList(prev), value)
		}
	}
	return params, nil
}

// readSecret reads one line from stdin, without echo on a terminal.
func readSecret(name string) (string, error) {
	fd := int(os.Stdin.Fd())
	if t, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
		fmt.Fprintf(os.Stderr, "%s: ", name)
		quiet := *t
		quiet.Lflag &^= unix.ECHO
		if unix.IoctlSetTermios(fd, unix.TCSETS, &quiet) == nil {
			defer func() {
				_ = unix.IoctlSetTermios(fd, unix.TCSETS, t)
				fmt.Fprintln(os.Stderr)
			}()
		}
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func asList(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}
