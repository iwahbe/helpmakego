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
	"sync"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"golang.org/x/mod/modfile"
)

// Find the set of files that are depended on by the package at root.
func Find(ctx context.Context, root string, testPaths, modFiles bool) ([]string, error) {
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

	if modFiles {
		(*sync.Map)(modules).Range(func(_, m any) bool {
			errs = append(errs, m.(module).addRootFiles(files))
			return true
		})
		if workspace != nil {
			errs = append(errs, workspace.addRootFiles(files))
		}
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

func findPackages(ctx context.Context, root string, includeTests bool) (iter.Seq2[*build.Package, error], *modules, *goWorkspace, error) {
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
		log.Info(ctx, "Added replace", log.Attr("from", r.Old.Path), log.Attr("to", r.New.Path))
		replaces[r.Old.Path] = filepath.Join(goMod.rootDir, r.New.Path) // Resolve to a better path
	}

	// Find the go.work, if any and if its not disabled.
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

	incoming := make(chan *build.Package, 50)

	ctx, cancel := context.WithCancelCause(ctx)

	_replaces := make([]replace, 0, len(replaces))
	for k, v := range replaces {
		_replaces = append(_replaces, replace{
			from: k, to: v,
		})
	}
	slices.SortFunc(_replaces, func(a, b replace) int {
		// this is a reverse sort on .from
		return strings.Compare(b.from, a.from)
	})

	finder := packageFinder{
		replaces:     _replaces,
		includeTests: includeTests,
		modules:      &modules,
		cancel:       cancel,
		dst:          incoming,
	}

	finder.wg.Add(1)
	go finder.findPackages(ctx, root, "")

	// done represents that package finding is still ongoing.
	//
	// When done is closed, it indicates that the search should stop
	go func() { finder.wg.Wait(); cancel(finishedCleanly{}) }()

	return func(yield func(*build.Package, error) bool) {
		// Make sure to avoid leaking the cancel request.
		defer cancel(nil)

		for {
			select {
			case pkg := <-incoming:
				if !yield(pkg, nil) {
					return
				}
			case <-ctx.Done():
				err := context.Cause(ctx)
				_, clean := err.(finishedCleanly)
				if !clean {
					yield(nil, err)
				}
				return
			}

		}
	}, &modules, goWork, nil
}

type finishedCleanly struct{}

func (finishedCleanly) Error() string { return "finished cleanly" }

type packageFinder struct {
	// Global state - does not change over time

	// replaces must be sorted (longest to shortest) on .from so a linear search will
	// pick up the correct module first.
	replaces     []replace
	includeTests bool

	// Local state, may mutate and thus must be safe to mutate in parallel.

	// modules is a map from directory names to their enclosing go module
	modules *modules

	// seen is a cache of modules already processed.
	seen sync.Map // Map of string -> struct{}

	dst chan<- *build.Package

	wg sync.WaitGroup

	cancel func(error)
}

type replace struct{ from, to string }

// A lookup table from directory names to the go module they represent.
//
// The underlying map is of string -> module
type modules sync.Map

type module struct {
	file    *modfile.File
	rootDir string
}

func (m *modules) findGoMod(ctx context.Context, root string) (mod module, err error) {
	if m == nil {
		panic("m should not be nil")
	}

	log.Debug(ctx, "Searching for go.mod", log.Attr("root", root))
	goModDir := root
	var goModBytes []byte
	for {
		log.Debug(ctx, "Searching for go.mod", log.Attr("haystack", goModDir))
		// Check the cache
		if mod, ok := (*sync.Map)(m).Load(root); ok {
			return mod.(module), nil
		}

		// Cache this dir to the module we eventually found.
		defer func(path string) {
			if err == nil {
				(*sync.Map)(m).Store(path, mod)
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

func (m *modules) findGoWork(ctx context.Context, root string) (mod *goWorkspace, err error) {
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

func (pf *packageFinder) findPackages(ctx context.Context, target string, pkgName string) {
	log.Debug(ctx, "searching for imports of", log.Attr("target", target))
	// Decrement the wait grounp associated with this function
	// call.
	//
	// Each call to findPackages should have called pf.wg.Add(1) before starting
	// findPackages in a background thread.
	defer pf.wg.Done()

	goMod, err := pf.modules.findGoMod(ctx, target)
	if err != nil {
		log.Debug(ctx, "failed to find go.mod for", log.Attr("target", target))
		pf.cancel(err)
		return
	}

	pkg, err := build.Default.ImportDir(target, 0)
	if err != nil {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			pf.cancel(fmt.Errorf("referenced package %q was not found: expected to be at %q", pkgName, target))
			return
		}

		pf.cancel(fmt.Errorf("cannot import dir: %w", err))
		return
	}
	pf.dst <- pkg

	searchImport := func(_import string) {
		if _, ok := pf.seen.LoadOrStore(_import, struct{}{}); ok {
			log.Debug(ctx, "Skipping repeated import", log.Attr("module", _import))
			return
		}
		rest, isInModule := moduleCovers(_import, goMod.file.Module.Mod.Path)
		if !isInModule {
			if replaceTarget, ok := pf.fromReplace(_import); ok {
				log.Debug(ctx, "Replacing import",
					log.Attr("from", _import), log.Attr("to", replaceTarget))
				pf.wg.Add(1)
				go pf.findPackages(ctx, replaceTarget, _import)
				return
			} else {
				log.Debug(ctx, "Skipping foreign import", log.Attr("module", _import))
				return
			}
		}
		pf.wg.Add(1)
		go pf.findPackages(ctx, filepath.Join(goMod.rootDir, rest), _import)
	}

	log.Debug(ctx, "finding transitive imports",
		log.Attr("target", target),
		log.Attr("imports", pkg.Imports),
	)

	// Check to see if we should stop.
	select {
	case <-ctx.Done():
		return
	default:
	}

	for _, _import := range pkg.Imports {
		searchImport(_import)
	}

	if pf.includeTests {
		for _, _import := range pkg.TestImports {
			searchImport(_import)
		}
		for _, _import := range pkg.XTestImports {
			searchImport(_import)
		}
	}
}

func (pf *packageFinder) fromReplace(_import string) (string, bool) {
	for _, replace := range pf.replaces {
		rest, ok := moduleCovers(_import, replace.from)
		if !ok {
			continue
		}
		return filepath.Join(replace.to, rest), true
	}
	return "", false
}

// moduleCovers should be used to check if _import should be covered by the module path from.
//
// It will return the non-covered suffix and true if _import should be covered by from.
// It will return ("", false) otherwise.
//
// This function is necessary (as opposed to [strings.CutPrefix]) to handle distinguishing
// between moduleCovers such as:
//
//	k8s.io/api => ./staging/src/k8s.io/api
//	k8s.io/apiextensions-apiserver => ./staging/src/k8s.io/apiextensions-apiserver
//	k8s.io/apimachinery => ./staging/src/k8s.io/apimachinery
//
// All prefixes start with "k8s.io/api", which is also a valid replace.
func moduleCovers(_import, from string) (string, bool) {
	chomp := func(path string) (string, string) {
		i := strings.IndexByte(path, '/')
		if i < 0 {
			return path, ""
		}
		return path[:i], path[i+1:]
	}

	for from != "" {
		var c1, c2 string
		c1, _import = chomp(_import)
		c2, from = chomp(from)
		if c1 != c2 {
			return "", false
		}
	}
	return _import, true
}
