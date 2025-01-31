package display

import (
	"context"
	"log/slog"
	"path/filepath"
)

func Relative(ctx context.Context, wd string, paths []string) []string {
	relativePaths := make([]string, len(paths))
	for i, path := range paths {
		relativePaths[i] = makeRelative(ctx, wd, path)
	}
	return relativePaths
}

func makeRelative(ctx context.Context, wd, path string) string {
	relPath, err := filepath.Rel(wd, path)
	if err == nil {
		return relPath
	}
	slog.WarnContext(ctx, "Unable to get relative path",
		slog.String("basepath", wd),
		slog.String("targetpath", path))
	return path
}
