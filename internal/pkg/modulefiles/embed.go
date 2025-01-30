package modulefiles

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

func expandEmbeds(ctx context.Context, root string, embeds []string) ([]string, error) {
	estimate := 0
	for _, embed := range embeds {
		estimate += len(embed)
	}
	files := make([]string, 0, estimate)
	var errs []error
	for _, glob := range embeds {
		f, err := expandEmbed(ctx, os.DirFS(root), glob)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		files = append(files, f...)
	}
	return files, errors.Join(errs...)
}

// expandEmbed expands a go:embed glob into the files it describes.
//
// From https://pkg.go.dev/embed:
//
// The //go:embed directive accepts multiple space-separated patterns for brevity, but it
// can also be repeated, to avoid very long lines when there are many patterns. The
// patterns are interpreted relative to the package directory containing the source
// file. The path separator is a forward slash, even on Windows systems. Patterns may not
// contain ‘.’ or ‘..’ or empty path elements, nor may they begin or end with a slash. To
// match everything in the current directory, use ‘*’ instead of ‘.’. To allow for naming
// files with spaces in their names, patterns can be written as Go double-quoted or
// back-quoted string literals.
func expandEmbed(ctx context.Context, dir fs.FS, embed string) ([]string, error) {
	if strings.HasPrefix(embed, `"`) ||
		strings.HasPrefix(embed, "`") {
		var err error
		embed, err = strconv.Unquote(embed)
		if err != nil {
			return nil, fmt.Errorf("invalid embed - failed to parse string: %w", err)
		}
	}

	if embed == "*" {
		return embedDir(ctx, dir, ".")
	}

	matches, err := fs.Glob(dir, embed)
	if err != nil {
		return nil, fmt.Errorf("invalid embed - invalid glob: %w", err)
	}

	var files []string
	var errs []error
	for _, match := range matches {
		f, err := dir.Open(match)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not read %q: %w", match, err))
			continue
		}
		s, err := f.Stat()
		if err != nil {
			errs = append(errs, fmt.Errorf("could not get FS info on %q: %w", match, err))
			continue
		}
		if s.IsDir() {
			dirEntries, err := embedDir(ctx, dir, match)
			files = append(files, dirEntries...)
			errs = append(errs, err)
		} else {
			files = append(files, match)
		}
	}

	return files, errors.Join(errs...)
}

// embedDir expands an entire directory, according to go:embed.
//
// From https://pkg.go.dev/embed:
//
// If a pattern names a directory, all files in the subtree rooted at that directory are
// embedded (recursively), except that files with names beginning with ‘.’ or ‘_’ are
// excluded. So the variable in the above example is almost equivalent to:
func embedDir(_ context.Context, root fs.FS, dir string) ([]string, error) {
	var files []string
	err := fs.WalkDir(root, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(path, ".") || strings.HasPrefix(path, "_") {
			return fs.SkipDir
		}
		// Make doesn't play well with directories, so we skip these.
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})

	return files, err
}
