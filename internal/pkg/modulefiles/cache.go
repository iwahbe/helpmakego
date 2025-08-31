package modulefiles

import (
	"context"
	"go/build"
	"sync"
)

type Cache struct {
	modules  *sync.Map // map[lookupKey]*modules
	packages cachedImporter
	modRoot  string
}

func NewCache(ctx context.Context, pkgRoot string) (Cache, error) {
	c := Cache{
		modules:  new(sync.Map),
		packages: cachedImporter{packages: new(sync.Map)},
	}
	modRoot, err := c.getModules(lookupKey{}).findGoMod(ctx, pkgRoot)
	c.modRoot = modRoot.rootDir
	return c, err
}

type cachedImporter struct{ packages *sync.Map }

func (c cachedImporter) ImportDir(dir string, mode build.ImportMode) (*build.Package, error) {
	type (
		key struct {
			dir  string
			mode build.ImportMode
		}
		value struct {
			pkg *build.Package
			err error
		}
	)

	k := key{dir, mode}
	v, ok := c.packages.Load(k)
	if !ok {
		pkg, err := build.Default.ImportDir(dir, mode)
		v, _ = c.packages.LoadOrStore(k, value{pkg, err})
	}

	val := v.(value)
	return val.pkg, val.err

}

func (c Cache) getModules(key lookupKey) *modules {
	k, ok := c.modules.Load(key)
	if ok {
		return k.(*modules)
	}
	k, _ = c.modules.LoadOrStore(key, new(modules))
	return k.(*modules)
}

func (c Cache) Find(ctx context.Context, pkg string, testPaths, modFiles, goWork bool) ([]string, error) {
	return findWithModules(ctx, pkg, testPaths, modFiles, goWork, c.getModules(lookupKey{
		test: testPaths,
		mod:  modFiles,
		work: goWork,
	}), c.packages)
}

func (c Cache) ModuleRoot() string { return c.modRoot }

// Find the module root of a given package directory.
//
// FindModuleRoot does not share a cache, and should only be used to support connecting
// to a daemon.
func FindModuleRoot(ctx context.Context, pkgRoot string) (string, error) {
	var modules modules
	goMod, err := modules.findGoMod(ctx, pkgRoot)
	return goMod.rootDir, err
}

type lookupKey struct{ test, mod, work bool }
