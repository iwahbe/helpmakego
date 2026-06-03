package modulefiles

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"github.com/iwahbe/helpmakego/internal/pkg/vcs"
	"golang.org/x/mod/modfile"
	xmodule "golang.org/x/mod/module"
)

// discoverFunc resolves a directory to the version control repository that
// contains it. [vcs.Discover] satisfies it; tests substitute a fake.
type discoverFunc func(ctx context.Context, dir string) (vcs.Repo, error)

// importGraph records, for each package directory, the imports it declares.
//
// It is written concurrently while packages are discovered and read once
// discovery is complete.
type importGraph struct {
	mu    sync.Mutex
	edges map[string][]graphEdge
}

// graphEdge is a single import declared by a package.
type graphEdge struct {
	// importPath is the path as written in the source.
	importPath string
	// dir is the local directory that provides the import, or "" when the import
	// is foreign (an external module).
	dir string
}

func newImportGraph() *importGraph {
	return &importGraph{edges: map[string][]graphEdge{}}
}

func (g *importGraph) add(importer, importPath, dir string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.edges[importer] = append(g.edges[importer], graphEdge{importPath: importPath, dir: dir})
}

func (g *importGraph) snapshot() map[string][]graphEdge {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string][]graphEdge, len(g.edges))
	maps.Copy(out, g.edges)
	return out
}

// affectedPackages returns the directories of the packages that could have
// changed since ref.
//
// A package is affected if one of its contributing files changed, if it imports
// an external module whose go.mod entry changed, or if it transitively imports
// such a package. Local replaces into a separate repository are diffed in that
// repository from the revision the requiring go.mod pinned at ref.
func affectedPackages(
	ctx context.Context, root, ref string, discover discoverFunc,
	pkgFiles map[string]map[string]struct{}, graph *importGraph, modules *modules,
) (map[string]struct{}, error) {
	repo, err := discover(ctx, root)
	if err != nil {
		return nil, err
	}

	changedList, err := repo.ChangedFiles(ctx, ref)
	if err != nil {
		return nil, err
	}
	changed := make(map[string]struct{}, len(changedList))
	for _, file := range changedList {
		changed[file] = struct{}{}
	}

	entry, err := modules.findGoMod(ctx, root)
	if err != nil {
		return nil, err
	}

	// Replaces into a separate repository are invisible to the main diff, so diff
	// each one in its own repository and fold the results in. Replacements we
	// cannot diff precisely are included wholesale.
	crossChanged, conservative := crossRepoChanges(ctx, repo, discover, ref, entry)
	for _, file := range crossChanged {
		changed[file] = struct{}{}
	}

	seeds := map[string]struct{}{}

	// Seed with packages whose own files changed.
	for dir, files := range pkgFiles {
		if _, ok := changed[dir]; ok || underAny(dir, conservative) {
			seeds[dir] = struct{}{}
			continue
		}
		for file := range files {
			if _, ok := changed[file]; ok {
				seeds[dir] = struct{}{}
				break
			}
		}
	}

	// Seed with packages importing an external module whose go.mod entry changed.
	changedModules, err := changedModulePaths(ctx, repo, ref, modules)
	if err != nil {
		return nil, err
	}
	if len(changedModules) > 0 {
		for importer, edges := range graph.snapshot() {
			for _, edge := range edges {
				if edge.dir == "" && coversAny(edge.importPath, changedModules) {
					seeds[importer] = struct{}{}
					break
				}
			}
		}
	}

	return closeOverImporters(seeds, graph), nil
}

// crossRepoChanges diffs every local replace whose target lives in a different
// repository than the entry module.
//
// The diff is anchored at the revision the requiring go.mod pinned at ref: the
// require version is translated to a VCS tag or, for a pseudo-version, the commit
// it encodes. A replace that cannot be diffed precisely is returned as a
// conservative directory whose packages are all treated as changed.
func crossRepoChanges(
	ctx context.Context, repo vcs.Repo, discover discoverFunc, ref string, entry module,
) (changed []string, conservative []string) {
	oldRequire, anchored := requireVersionsAtRef(ctx, repo, ref, entry)

	for _, r := range entry.file.Replace {
		if !modfile.IsDirectoryPath(r.New.Path) {
			continue
		}
		dir := filepath.Join(entry.rootDir, r.New.Path)

		subRepo, err := discover(ctx, dir)
		if err != nil {
			log.Warn(ctx, "cannot resolve repository for local replace; treating all of its files as changed",
				log.Attr("module", r.Old.Path), log.Attr("dir", dir), log.Attr("error", err.Error()))
			conservative = append(conservative, dir)
			continue
		}
		if subRepo.Root() == repo.Root() {
			continue // Same repository; already covered by the main diff.
		}

		version := oldRequire[r.Old.Path]
		switch {
		case !anchored:
			// The entry go.mod could not be read at ref, so there is no anchor.
			conservative = append(conservative, dir)
			continue
		case version == "":
			// Not required at ref: a brand new dependency the module diff handles.
			continue
		}

		rev, err := versionToRevision(version)
		if err != nil {
			log.Warn(ctx, "cannot translate module version to a revision; treating all of its files as changed",
				log.Attr("module", r.Old.Path), log.Attr("version", version), log.Attr("error", err.Error()))
			conservative = append(conservative, dir)
			continue
		}
		files, err := subRepo.ChangedFiles(ctx, rev)
		if err != nil {
			log.Warn(ctx, "cannot diff local replace repository; treating all of its files as changed",
				log.Attr("module", r.Old.Path), log.Attr("dir", dir),
				log.Attr("revision", rev), log.Attr("error", err.Error()))
			conservative = append(conservative, dir)
			continue
		}
		changed = append(changed, files...)
	}
	return changed, conservative
}

// requireVersionsAtRef reads the entry module's go.mod as of ref and maps each
// required module path to its version. The second result is false when the
// go.mod could not be read, meaning no revision can be anchored.
func requireVersionsAtRef(
	ctx context.Context, repo vcs.Repo, ref string, entry module,
) (map[string]string, bool) {
	rel, err := filepath.Rel(repo.Root(), filepath.Join(entry.rootDir, "go.mod"))
	if err != nil {
		return nil, false
	}
	bytes, err := repo.ReadFileAtRef(ctx, ref, rel)
	if err != nil {
		return nil, false
	}
	file, err := modfile.Parse("go.mod", bytes, nil)
	if err != nil {
		return nil, false
	}
	versions := make(map[string]string, len(file.Require))
	for _, r := range file.Require {
		versions[r.Mod.Path] = r.Mod.Version
	}
	return versions, true
}

// versionToRevision translates a module version into the VCS revision it refers
// to: the commit encoded in a pseudo-version, or the tag named by a released
// version.
func versionToRevision(version string) (string, error) {
	if xmodule.IsPseudoVersion(version) {
		return xmodule.PseudoVersionRev(version)
	}
	// A tag carries no build metadata, so drop suffixes like "+incompatible".
	if i := strings.IndexByte(version, '+'); i >= 0 {
		version = version[:i]
	}
	return version, nil
}

// underAny reports whether dir is one of roots or nested within one of them.
func underAny(dir string, roots []string) bool {
	for _, root := range roots {
		if dir == root || strings.HasPrefix(dir, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// changedModulePaths returns the set of module paths whose go.mod resolution
// differs between ref and the working tree across every discovered module.
func changedModulePaths(
	ctx context.Context, repo vcs.Repo, ref string, modules *modules,
) (map[string]struct{}, error) {
	changed := map[string]struct{}{}
	seen := map[string]struct{}{}
	var errs []error

	for _, v := range (*sync.Map)(modules).Range {
		mod := v.(module)
		if _, ok := seen[mod.rootDir]; ok {
			continue
		}
		seen[mod.rootDir] = struct{}{}

		rel, err := filepath.Rel(repo.Root(), filepath.Join(mod.rootDir, "go.mod"))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if strings.HasPrefix(rel, "..") {
			// The module lives in a different repository; its changes are tracked
			// by diffing that repository's files, not its go.mod here.
			continue
		}

		var old map[string]string
		switch oldBytes, err := repo.ReadFileAtRef(ctx, ref, rel); {
		case errors.Is(err, os.ErrNotExist):
			// The module did not exist at ref; every current requirement is new.
		case err != nil:
			errs = append(errs, err)
			continue
		default:
			oldFile, err := modfile.Parse("go.mod", oldBytes, nil)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			old = moduleResolutions(oldFile)
		}

		// Locally replaced modules resolve to a directory whose changes are tracked
		// by file diffs, so a go.mod version change must not flag them here.
		local := localReplacedPaths(mod.file)
		for path := range diffResolutions(old, moduleResolutions(mod.file)) {
			if _, ok := local[path]; ok {
				continue
			}
			changed[path] = struct{}{}
		}
	}

	return changed, errors.Join(errs...)
}

// localReplacedPaths returns the set of module paths that f replaces with a local
// directory.
func localReplacedPaths(f *modfile.File) map[string]struct{} {
	local := map[string]struct{}{}
	for _, r := range f.Replace {
		if modfile.IsDirectoryPath(r.New.Path) {
			local[r.Old.Path] = struct{}{}
		}
	}
	return local
}

// moduleResolutions maps each module path required by f to a token describing how
// it resolves. A change in the token means the dependency's compiled output could
// have changed.
func moduleResolutions(f *modfile.File) map[string]string {
	res := make(map[string]string, len(f.Require))
	for _, r := range f.Require {
		res[r.Mod.Path] = "v=" + r.Mod.Version
	}
	// A replace overrides the requirement, so it is applied second.
	for _, r := range f.Replace {
		res[r.Old.Path] = "r=" + r.New.Path + "@" + r.New.Version
	}
	return res
}

// diffResolutions returns the module paths whose resolution token differs between
// old and new, including paths present in only one of them.
func diffResolutions(old, new map[string]string) map[string]struct{} {
	changed := map[string]struct{}{}
	for path, token := range old {
		if new[path] != token {
			changed[path] = struct{}{}
		}
	}
	for path, token := range new {
		if old[path] != token {
			changed[path] = struct{}{}
		}
	}
	return changed
}

// coversAny reports whether importPath belongs to any of the given module paths.
func coversAny(importPath string, modulePaths map[string]struct{}) bool {
	for modulePath := range modulePaths {
		if _, ok := moduleCovers(importPath, modulePath); ok {
			return true
		}
	}
	return false
}

// closeOverImporters expands seeds to include every package that transitively
// imports a seed package.
func closeOverImporters(seeds map[string]struct{}, graph *importGraph) map[string]struct{} {
	importers := map[string][]string{}
	for importer, edges := range graph.snapshot() {
		for _, edge := range edges {
			if edge.dir != "" {
				importers[edge.dir] = append(importers[edge.dir], importer)
			}
		}
	}

	affected := make(map[string]struct{}, len(seeds))
	stack := make([]string, 0, len(seeds))
	for dir := range seeds {
		affected[dir] = struct{}{}
		stack = append(stack, dir)
	}
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, importer := range importers[dir] {
			if _, ok := affected[importer]; !ok {
				affected[importer] = struct{}{}
				stack = append(stack, importer)
			}
		}
	}
	return affected
}
