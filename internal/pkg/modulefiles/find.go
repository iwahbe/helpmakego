package modulefiles

import (
	"context"
	"errors"
	"fmt"
	"go/build"
	"iter"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"golang.org/x/mod/modfile"
)

// Find the set of files that are depended on by the package at root.
func Find(ctx context.Context, root string, testPaths bool) ([]string, error) {
	var errs []error

	files := map[string]struct{}{}
	if os.Getenv("GO111MODULE") == "off" {
		return nil, fmt.Errorf("Go modules disabled")
	}

	packages, modules, workspace, err := findPackages(ctx, root, testPaths)
	if err != nil {
		return nil, err
	}
	for pkg, err := range packages {
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
	if workspace != nil {
		errs = append(errs, workspace.addRootFiles(files))
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

func findPackages(ctx context.Context, root string, includeTests bool) (iter.Seq2[*build.Package, error], modules, *goWorkspace, error) {
	modules := modules{}
	goMod, err := modules.findGoMod(ctx, root)
	if err != nil {
		log.Debug(ctx, "unable to find initial go.mod")
		return nil, nil, nil, err
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

	var goWork *goWorkspace
	if os.Getenv("GOWORK") == "off" {
		log.Debug(ctx, "Go workspaces explicitly disabled")
	} else {
		goWork, err = modules.findGoWork(ctx, root)
		if errors.Is(err, noGoWorkFound) {
			log.Debug(ctx, "no go.work found above %s")
		} else if err != nil {
			return nil, nil, nil, err
		} else {
			// Apply replaces from go.work
			for _, r := range goWork.file.Replace {
				// We only follow local replaces
				if !modfile.IsDirectoryPath(r.New.Path) {
					continue
				}
				replaces[r.Old.Path] = filepath.Join(goWork.rootDir, r.New.Path) // Resolve to a better path
			}

			// Apply `use` statements
			for _, u := range goWork.file.Use {
				modDir := path.Clean(path.Join(goWork.rootDir, u.Path)) // modDir is where we expect the module to live
				mod, err := modules.findGoMod(ctx, modDir)
				if err != nil {
					log.Error(ctx, "unable to find module in go.work",
						log.Attr("module", modDir),
						log.Attr("workspace", goWork.rootDir),
					)
					continue
				}
				// For our purposes, each `use` statement resolves like a replace statement.
				replaces[mod.file.Module.Mod.Path] = modDir
			}
		}
	}

	finder := packageFinder{
		replaces:     replaces,
		includeTests: includeTests,
		ctx:          ctx,
		modules:      modules,

		seen: map[string]struct{}{},
	}

	return finder.findPackages(root), modules, goWork, nil
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

	log.Info(ctx, "Searching for go.mod", log.Attr("root", root))
	goModDir := root
	var goModBytes []byte
	for {
		log.Debug(ctx, "Searching for go.mod", log.Attr("haystack", goModDir))
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

type goWorkspace struct {
	file    *modfile.WorkFile
	rootDir string
}

func (m modules) findGoWork(ctx context.Context, root string) (mod *goWorkspace, err error) {
	if m == nil {
		panic("m should not be nil")
	}

	log.Info(ctx, "Searching for go.work", log.Attr("root", root))
	dir, err := m.findGoMod(ctx, root) // We root our search in the enclosing go.mod's dir
	if err != nil {
		return nil, err
	}
	goWorkDir := dir.rootDir
	var goWorkBytes []byte
	for {
		log.Debug(ctx, "Searching for go.work", log.Attr("haystack", goWorkDir))

		if b, err := os.ReadFile(filepath.Join(goWorkDir, "go.work")); err == nil {
			goWorkBytes = b
			break
		} else if os.IsNotExist(err) {
			goWorkDir = filepath.Dir(goWorkDir)
			if goWorkDir == string(filepath.Separator) || goWorkDir == "." {
				return nil, noGoWorkFound
			}
		} else {
			return nil, err
		}
	}

	goWork, err := modfile.ParseWork("go.mod", goWorkBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", filepath.Join(goWorkDir, "go.work"), err)
	}
	return &goWorkspace{file: goWork, rootDir: goWorkDir}, nil
}

func (m goWorkspace) addRootFiles(files map[string]struct{}) error {
	// Add go.work & go.work.sum
	goWorkPath := filepath.Join(m.rootDir, "go.work")
	goWorkSumPath := filepath.Join(m.rootDir, "go.work.sum")
	files[goWorkPath] = struct{}{}
	if _, err := os.Stat(goWorkSumPath); err == nil {
		files[goWorkSumPath] = struct{}{}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not check if go.work.sum exists: %w", err)
	}
	return nil
}

var noGoWorkFound = errors.New("no go.work found")

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
		log.Debug(pf.ctx, "searching for imports of", log.Attr("target", target))

		goMod, err := pf.modules.findGoMod(pf.ctx, target)
		if err != nil {
			log.Debug(pf.ctx, "failed to find go.mod for", log.Attr("target", target))
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
				log.Debug(pf.ctx, "Skipping repeated import", log.Attr("module", _import))
				return
			}
			pf.seen[_import] = struct{}{}
			rest, isInModule := strings.CutPrefix(_import, goMod.file.Module.Mod.Path)
			if !isInModule {
				if replaceTarget, ok := pf.fromReplace(_import); ok {
					log.Debug(pf.ctx, "Replacing import",
						log.Attr("from", _import), log.Attr("to", replaceTarget))
					pf.findPackages(replaceTarget)(yield)
					return
				} else {
					log.Debug(pf.ctx, "Skipping foreign import", log.Attr("module", _import))
					return
				}
			}
			pf.findPackages(filepath.Join(goMod.rootDir, rest))(yield)
		}

		log.Debug(pf.ctx, "finding transitive imports",
			log.Attr("target", target),
			log.Attr("imports", pkg.Imports),
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
