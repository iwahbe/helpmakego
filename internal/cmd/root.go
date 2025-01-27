package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
)

func Root() *cobra.Command {
	return &cobra.Command{
		Use:          "helpmakego [path-to-module]",
		Short:        "Find all files a Go module depends on - suitable for Make",
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var modRoot string
			if len(args) == 0 {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				modRoot = wd
			} else {
				modRoot = args[0]
			}

			modRoot, err := filepath.Abs(modRoot)
			if err != nil {
				return nil
			}

			slog.SetLogLoggerLevel(slog.LevelDebug)

			f, err := modulefiles.Find(cmd.Context(), modRoot, true /* include test files */)
			if err != nil {
				return err
			}

			_, err = fmt.Printf("%s\n", strings.Join(f, " "))
			return err
		},
	}
}
