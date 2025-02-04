package modulefiles

import (
	"context"
	"errors"
	"fmt"
	"go/build"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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

	modules := modules{}
	for pkg, err := range findPackages(ctx, modules, root, testPaths) {
		if err != nil {
			errs = append(errs, err)
		}
		if pkg == nil {
			continue
		}

		errs = append(errs, importPackage(ctx, pkg, testPaths, func(fileName string) {
			files[filepath.Join(pkg.Dir, fileName)] = struct{}{}
		}))
	}

	for _, m := range modules {
		errs = append(errs, m.addRootFiles(files))
	}

	sortedFiles := make([]string, 0, len(files))
	for file := range files {
		sortedFiles = append(sortedFiles, file)
	}
	slices.Sort(sortedFiles)
	return sortedFiles, errors.Join(errs...)
}

func importPackage(ctx context.Context, pkg *build.Package, includeTests bool, addFile addFile) error {
	var errs []error
	errs = append(errs, expandEmbeds(ctx, os.DirFS(pkg.Dir), pkg.EmbedPatterns, addFile))
	if includeTests {
		// Include test files
		applyNested(addFile,
			pkg.TestGoFiles,
			pkg.XTestGoFiles,
		)
		// Include test embeds
		errs = append(errs, expandEmbeds(ctx, os.DirFS(pkg.Dir), pkg.TestEmbedPatterns, addFile))
		errs = append(errs, expandEmbeds(ctx, os.DirFS(pkg.Dir), pkg.XTestEmbedPatterns, addFile))
	}

	applyNested(addFile,
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
	)

	return errors.Join(errs...)
}

type addFile = func(fileName string)

func findPackages(ctx context.Context, modules modules, root string, includeTests bool) iter.Seq2[*build.Package, error] {
	goMod, err := modules.findGoMod(ctx, root)
	if err != nil {
		slog.DebugContext(ctx, "unable to find initial go.mod")
		return func(yield func(*build.Package, error) bool) {
			yield(nil, err)
		}
	}
	// Find the go.mod
	replaces := make(map[string]string, len(goMod.file.Replace))
	for _, r := range goMod.file.Replace {
		// We only follow local replaces
		if !modfile.IsDirectoryPath(r.New.Path) {
			continue
		}
		replaces[r.Old.Path] = filepath.Join(goMod.rootDir, r.New.Path) // Resolve to a better path
	}

	finder := packageFinder{
		replaces:     replaces,
		includeTests: includeTests,
		ctx:          ctx,
		modules:      modules,

		seen: map[string]struct{}{},
	}

	return finder.findPackages(root)
}

type packageFinder struct {
	ctx context.Context

	// modules is a map from directory names to their enclosing go module
	modules modules

	replaces     map[string]string
	includeTests bool

	// seen is a cache of modules already processed.
	seen map[string]struct{}

	// done indicates the traversal should abort.
	done bool
}

type modules map[string]module

type module struct {
	file    *modfile.File
	rootDir string
}

func (m modules) findGoMod(ctx context.Context, root string) (mod module, err error) {
	if m == nil {
		panic("m should not be nil")
	}

	slog.InfoContext(ctx, "Searching for go.mod", slog.String("root", root))
	goModDir := root
	var goModBytes []byte
	for {
		slog.DebugContext(ctx, "Searching for go.mod", slog.String("haystack", goModDir))
		// Check the cache
		if mod, ok := m[root]; ok {
			return mod, nil
		}

		// Cache this dir to the module we eventually found.
		defer func(path string) {
			if err == nil {
				m[path] = mod
			}
		}(goModDir)

		if b, err := os.ReadFile(filepath.Join(goModDir, "go.mod")); err == nil {
			goModBytes = b
			break
		} else if os.IsNotExist(err) {
			goModDir = filepath.Dir(goModDir)
			if goModDir == string(filepath.Separator) || goModDir == "." {
				return module{}, errors.New("no go.mod file found")
			}
		} else {
			return module{}, err
		}
	}

	goMod, err := modfile.Parse("go.mod", goModBytes, nil)
	if err != nil {
		return module{}, fmt.Errorf("could not parse %s: %w", filepath.Join(goModDir, "go.mod"), err)
	}
	return module{file: goMod, rootDir: goModDir}, nil
}

func (m module) addRootFiles(files map[string]struct{}) error {
	// Add go.{mod,sum}
	goModPath := filepath.Join(m.rootDir, "go.mod")
	goSumPath := filepath.Join(m.rootDir, "go.sum")
	files[goModPath] = struct{}{} // goModPath must exist, since findGoMod returned without an error
	if _, err := os.Stat(goSumPath); err == nil {
		files[goSumPath] = struct{}{}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not check if go.sum exists: %w", err)
	}
	return nil
}

func (pf *packageFinder) findPackages(target string) func(yield func(*build.Package, error) bool) {
	return func(yield func(*build.Package, error) bool) {
		slog.DebugContext(pf.ctx, "searching for imports of", slog.String("target", target))

		goMod, err := pf.modules.findGoMod(pf.ctx, target)
		if err != nil {
			slog.DebugContext(pf.ctx, "failed to find go.mod for", slog.String("target", target))
			yield(nil, err)
			pf.done = true
			return
		}

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
			rest, isInModule := strings.CutPrefix(_import, goMod.file.Module.Mod.Path)
			if !isInModule {
				if replaceTarget, ok := pf.fromReplace(_import); ok {
					slog.DebugContext(pf.ctx, "Replacing import",
						slog.String("from", _import), slog.String("to", replaceTarget))
					pf.findPackages(replaceTarget)(yield)
					return
				} else {
					slog.DebugContext(pf.ctx, "Skipping foreign import", slog.String("module", _import))
					return
				}
			}
			pf.findPackages(filepath.Join(goMod.rootDir, rest))(yield)
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
