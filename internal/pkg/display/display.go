package display

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
)

func Relative(ctx context.Context, wd string, paths []string) []string {
	relativePaths := make([]string, len(paths))
	for i, path := range paths {
		relativePaths[i] = escapePath(ctx, makeRelative(ctx, wd, path))
	}
	return relativePaths
}

func makeRelative(ctx context.Context, wd, path string) string {
	relPath, err := filepath.Rel(wd, path)
	if err == nil {
		return relPath
	}
	log.Warn(ctx, "Unable to get relative path",
		log.Attr("basepath", wd),
		log.Attr("targetpath", path))
	return filepath.Clean(path)
}

// escapePath tries to make a path printable without error.
func escapePath(ctx context.Context, path string) string {
	if !strings.ContainsAny(path, `$"' `) {
		// No escaping necessary
		return path
	}

	// If the string needs to be escaped, but doesn't contain any `'`, then we can
	// just use those.
	if !strings.ContainsRune(path, '\'') {
		return "'" + path + "'"
	}

	// Now we need to escape with just simple ".
	if !strings.ContainsRune(path, '"') {
		// If a case is not handled, then we will warn and make a best effort.
		if strings.ContainsRune(path, '$') {
			log.Warn(ctx, fmt.Sprintf(`Unable to fully escape path %q: contains a "$"`, path))
		}
		return `"` + path + `"`
	}

	// We are unable to escape the string, so warn and move on.
	log.Warn(ctx, fmt.Sprintf(`Unable to escape path %q: contains both "\"" and "'"`, path))
	return path
}
