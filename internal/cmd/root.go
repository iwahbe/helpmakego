package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iwahbe/helpmakego/internal/pkg/display"
	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
)

func Root() *cobra.Command {
	return &cobra.Command{
		Use:          "helpmakego [path-to-module]",
		Short:        "Find all files a Go module depends on - suitable for Make",
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

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
				return err
			}

			switch os.Getenv("LOG") {
			case "debug":
				slog.SetLogLoggerLevel(slog.LevelDebug)
			case "error":
				slog.SetLogLoggerLevel(slog.LevelError)
			case "info":
				slog.SetLogLoggerLevel(slog.LevelInfo)
			case "", "warn":
				slog.SetLogLoggerLevel(slog.LevelWarn)
			default:
				slog.SetLogLoggerLevel(slog.LevelWarn)
				slog.WarnContext(ctx, `invalid log level %q: valid options are "error", "warn", "info" and "debug"`)
			}

			paths, err := modulefiles.Find(ctx, modRoot, true /* include test files */)
			if err != nil {
				return err
			}

			if cwd, err := os.Getwd(); err == nil {
				paths = display.MakeRelative(ctx, cwd, paths)
			} else {
				slog.WarnContext(ctx, "os.Getwd() failed - displaying absolute paths")
			}

			_, err = fmt.Printf("%s\n", strings.Join(paths, " "))
			return err
		},
	}
}
