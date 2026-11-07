package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func keepCommand(use, short string, keep bool, done string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <library:id>...",
		Short: short,
		Long:  short + ".\nTracks are addressed as `library:id`, the library's id and the track's id joined by a colon.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := call(true, "cache.keep", map[string]any{"tracks": args, "keep": keep}, nil); err != nil {
				return err
			}
			if !jsonOutput {
				fmt.Printf("%s %d track(s)\n", done, len(args))
			}
			return nil
		},
	}
}

func init() {
	rootCmd.AddCommand(
		keepCommand("keep", "Keep server tracks on this device for offline play", true, "Keeping"),
		keepCommand("unkeep", "Let go of tracks kept by hand", false, "Released"),
	)
}
