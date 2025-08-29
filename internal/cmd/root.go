package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iwahbe/helpmakego/internal/pkg/display"
	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
)

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "helpmakego [path-to-package] [--test]",
		Short:        "Find all files a Go package depends on - suitable for Make",
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
	}
	cmd.AddCommand(deamon())

	includeTest := cmd.Flags().Bool("test", false, "include test files in the dependency analysis")
	outputJSON := cmd.Flags().Bool("json", false, "output source files as a a JSON array")
	absolutePaths := cmd.Flags().Bool("abs", false, "output absolute paths instead of relative paths")
	includeMod := cmd.Flags().Bool("mod", true, "include module files in the result")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		var pkgPath string
		if len(args) == 0 {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			pkgPath = wd
		} else {
			pkgPath = args[0]
		}

		pkgPath, err := filepath.Abs(pkgPath)
		if err != nil {
			return err
		}

		setLevel := func(level slog.Level) context.Context {
			return log.New(ctx, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: level,
			})))
		}

		switch os.Getenv("LOG") {
		case "debug":
			ctx = setLevel(slog.LevelDebug)
		case "error":
			ctx = setLevel(slog.LevelError)
		case "info":
			ctx = setLevel(slog.LevelInfo)
		case "", "warn":
			ctx = setLevel(slog.LevelWarn)
		default:
			ctx = setLevel(slog.LevelWarn)
			log.Warn(ctx, `invalid log level %q: valid options are "error", "warn", "info" and "debug"`)
		}

		paths, err := modulefiles.Find(ctx, pkgPath, *includeTest, *includeMod)
		if err != nil {
			return err
		}

		if !*absolutePaths {
			if cwd, err := os.Getwd(); err == nil {
				paths = display.Relative(ctx, cwd, paths)
			} else {
				log.Warn(ctx, "os.Getwd() failed - displaying absolute paths")
			}
		}

		if *outputJSON {
			err = json.NewEncoder(os.Stdout).Encode(paths)
		} else {
			_, err = fmt.Printf("%s\n", strings.Join(paths, " "))
		}
		return err
	}

	return cmd
}
