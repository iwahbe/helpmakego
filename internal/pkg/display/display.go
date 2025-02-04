package display

import (
	"context"
	"path/filepath"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
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
	log.Warn(ctx, "Unable to get relative path",
		log.Attr("basepath", wd),
		log.Attr("targetpath", path))
	return path
}
