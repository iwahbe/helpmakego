package modulefiles

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

func expandEmbeds(ctx context.Context, root fs.FS, embeds []string, addFile addFile) error {
	var errs []error
	for _, glob := range embeds {
		errs = append(errs, expandEmbed(ctx, root, glob, addFile))
	}
	return errors.Join(errs...)
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
func expandEmbed(ctx context.Context, dir fs.FS, embed string, addFile addFile) error {
	if strings.HasPrefix(embed, `"`) ||
		strings.HasPrefix(embed, "`") {
		var err error
		embed, err = strconv.Unquote(embed)
		if err != nil {
			return fmt.Errorf("invalid embed - failed to parse string: %w", err)
		}
	}

	if embed == "*" {
		return embedDir(ctx, dir, ".", addFile)
	}

	matches, err := fs.Glob(dir, embed)
	if err != nil {
		return fmt.Errorf("invalid embed - invalid glob: %w", err)
	}

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
			errs = append(errs, embedDir(ctx, dir, match, addFile))
		} else {
			addFile(match)
		}
	}

	return errors.Join(errs...)
}

// embedDir expands an entire directory, according to go:embed.
//
// From https://pkg.go.dev/embed:
//
// If a pattern names a directory, all files in the subtree rooted at that directory are
// embedded (recursively), except that files with names beginning with ‘.’ or ‘_’ are
// excluded. So the variable in the above example is almost equivalent to:
func embedDir(_ context.Context, root fs.FS, dir string, addFile addFile) error {
	return fs.WalkDir(root, dir, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil || dir == filePath {
			return err
		}

		if name := path.Base(filePath); strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Make doesn't play well with directories, so we skip these.
		if d.IsDir() {
			return nil
		}
		addFile(filePath)
		return nil
	})
}
