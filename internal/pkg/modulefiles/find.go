package modulefiles

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"go/build"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

// Find the set of files that are depended on by the package at root.
func Find(ctx context.Context, root string, testPaths bool) ([]string, error) {
	var errs []error

	files := map[string]struct{}{}
	if os.Getenv("GO111MODULE") == "off" {
		return nil, fmt.Errorf("Go modules disabled")
	}

	goModDir, goMod, err := findGoMod(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("Unable to discover Go module: %w", err)
	}

	{ // Add go.{mod,sum}
		goModPath := filepath.Join(goModDir, "go.mod")
		goSumPath := filepath.Join(goModDir, "go.sum")
		files[goModPath] = struct{}{} // goModPath must exist, since findGoMod returned without an error
		if _, err := os.Stat(goSumPath); err == nil {
			files[goSumPath] = struct{}{}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not check if go.sum exists: %w", err)
		}
	}

	for pkg, err := range findPackages(ctx, goMod, goModDir, root, testPaths) {
		if err != nil {
			errs = append(errs, err)
		}
		if pkg == nil {
			continue
		}

		pkgFiles, err := importPackage(ctx, pkg, testPaths)
		if err != nil {
			errs = append(errs, err)
		}

		for _, file := range pkgFiles {
			files[filepath.Join(pkg.Dir, file)] = struct{}{}
		}
	}

	sortedFiles := make([]string, 0, len(files))
	for file := range files {
		sortedFiles = append(sortedFiles, file)
	}
	slices.Sort(sortedFiles)
	return sortedFiles, errors.Join(errs...)
}

func findGoMod(ctx context.Context, root string) (string, *modfile.File, error) {
	slog.DebugContext(ctx, "Searching for go.mod", slog.String("root", root))
	goModDir := root
	var goModBytes []byte
	for {
		if b, err := os.ReadFile(filepath.Join(goModDir, "go.mod")); err == nil {
			goModBytes = b
			break
		} else if os.IsNotExist(err) {
			goModDir = filepath.Dir(goModDir)
			if goModDir == string(filepath.Separator) {
				return "", nil, errors.New("no go.mod file found")
			}
		} else {
			return "", nil, err
		}
	}

	goMod, err := modfile.Parse("go.mod", goModBytes, nil)
	if err != nil {
		return "", nil, fmt.Errorf("could not parse %s: %w", filepath.Join(goModDir, "go.mod"), err)
	}
	return goModDir, goMod, nil
}

func findPackages(ctx context.Context, goMod *modfile.File, goModRoot, root string, includeTests bool) iter.Seq2[*build.Package, error] {
	// Find the go.mod
	replaces := make(map[string]string, len(goMod.Replace))
	for _, r := range goMod.Replace {
		// We only follow local replaces
		if !modfile.IsDirectoryPath(r.New.Path) {
			continue
		}
		replaces[r.Old.Path] = r.New.Path // Resolve to a better path
	}

	finder := packageFinder{
		moduleName:    goMod.Module.Mod.Path,
		moduleRootDir: goModRoot,
		replaces:      replaces,
		includeTests:  includeTests,
		ctx:           ctx,

		seen: map[string]struct{}{},
	}

	return finder.findPackages(root)
}

type packageFinder struct {
	ctx           context.Context
	moduleName    string
	moduleRootDir string
	replaces      map[string]string
	includeTests  bool

	seen map[string]struct{}
	done bool
}

func (pf *packageFinder) findPackages(target string) func(yield func(*build.Package, error) bool) {
	return func(yield func(*build.Package, error) bool) {
		slog.DebugContext(pf.ctx, "searching for imports of", slog.String("target", target))
		pkg, err := build.Default.ImportDir(target, 0)
		if !yield(pkg, err) || err != nil {
			pf.done = true
			return
		}

		searchImport := func(_import string) {
			if _, ok := pf.seen[_import]; ok {
				slog.DebugContext(pf.ctx, "Skipping repeated import", slog.String("module", _import))
				return
			}
			pf.seen[_import] = struct{}{}
			rest, isInModule := strings.CutPrefix(_import, pf.moduleName)
			if !isInModule {
				if replaceTarget, ok := pf.fromReplace(_import); ok {
					slog.DebugContext(pf.ctx, "Replacing import",
						slog.String("from", _import), slog.String("to", replaceTarget))
					pf.findPackages(replaceTarget)(yield)
				} else {
					slog.DebugContext(pf.ctx, "Skipping foreign import", slog.String("module", _import))
					return
				}
			}
			pf.findPackages(filepath.Join(pf.moduleRootDir, rest))(yield)
		}

		slog.DebugContext(pf.ctx, "finding transitive imports",
			slog.String("target", target),
			slog.Any("imports", pkg.Imports),
		)

		for _, _import := range pkg.Imports {
			if searchImport(_import); pf.done {
				return
			}

		}

		if pf.includeTests {
			for _, _import := range pkg.TestImports {
				if searchImport(_import); pf.done {
					return
				}
			}
			for _, _import := range pkg.XTestImports {
				if searchImport(_import); pf.done {
					return
				}
			}
		}
	}
}

func (pf *packageFinder) fromReplace(_import string) (string, bool) {
	for mod, newPath := range pf.replaces {
		rest, ok := strings.CutPrefix(_import, mod)
		if !ok {
			continue
		}
		return filepath.Join(newPath, rest), true
	}
	return "", false
}

func importPackage(ctx context.Context, pkg *build.Package, includeTests bool) ([]string, error) {
	var errs []error
	embedPatterns, err := expandEmbeds(ctx, pkg.Dir, pkg.EmbedPatterns)
	errs = append(errs, err)
	var testEmbedPatterns, xTestEmbedPatterns []string
	if includeTests {
		testEmbedPatterns, err = expandEmbeds(ctx, pkg.Dir, pkg.TestEmbedPatterns)
		errs = append(errs, err)
		xTestEmbedPatterns, err = expandEmbeds(ctx, pkg.Dir, pkg.XTestEmbedPatterns)
		errs = append(errs, err)
	}
	return join(
		pkg.GoFiles,        // .go source files (excluding CgoFiles, TestGoFiles, XTestGoFiles)
		pkg.CgoFiles,       // .go source files that import "C"
		pkg.IgnoredGoFiles, // .go source files ignored for this build (including ignored _test.go files)
		pkg.InvalidGoFiles, // .go source files with detected problems (parse error, wrong package name, and so on)
		pkg.CFiles,         // .c source files
		pkg.CXXFiles,       // .cc, .cpp and .cxx source files
		pkg.MFiles,         // .m (Objective-C) source files
		pkg.HFiles,         // .h, .hh, .hpp and .hxx source files
		pkg.FFiles,         // .f, .F, .for and .f90 Fortran source files
		pkg.SFiles,         // .s source files
		pkg.SwigFiles,      // .swig files
		pkg.SwigCXXFiles,   // .swigcxx files
		pkg.SysoFiles,      // .syso system object files to add to archive
		embedPatterns,
		testEmbedPatterns,
		xTestEmbedPatterns,
	), errors.Join(errs...)
}

func join[T any](arrs ...[]T) []T {
	size := 0
	for _, v := range arrs {
		size += len(v)
	}
	i := 0
	dst := make([]T, size)
	for _, arr := range arrs {
		for _, e := range arr {
			dst[i] = e
			i++
		}
	}
	return dst
}

//go:embed *.go
var _ []byte

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
