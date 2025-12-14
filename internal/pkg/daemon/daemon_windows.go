//go:build windows

package daemon

import (
	"context"
	"errors"

	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
)

// The deamon.go implementation relies on unix specific features:
//
// - Setting PIDs
// - Sockets
//
// The we don't support any background server on windows, instead just calling
// [modulefiles.Find] directly in-process.

func Serve(ctx context.Context, pkgRoot string) error {
	return errors.New("daemon is not supported on Windows")
}

func Find(ctx context.Context, pkgRoot string, includeTests, includeMod, goWork bool) ([]string, error) {
	return modulefiles.Find(ctx, pkgRoot, includeTests, includeMod, goWork)
}
