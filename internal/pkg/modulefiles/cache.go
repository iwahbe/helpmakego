package modulefiles

import (
	"context"
	"sync"
)

type Cache struct {
	modules *sync.Map
	modRoot string
}

func NewCache(ctx context.Context, pkgRoot string) (Cache, error) {
	c := Cache{modules: new(sync.Map)}
	modRoot, err := c.getModules(lookupKey{}).findGoMod(ctx, pkgRoot)
	c.modRoot = modRoot.rootDir
	return c, err
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
	}))
}

func (c Cache) ModuleRoot() string { return c.modRoot }

// Find the module root of a given package directory.
//
// FindModuleRoot does not share a cache, and should only be used to support connecting
// to a deamon.
func FindModuleRoot(ctx context.Context, pkgRoot string) (string, error) {
	var modules modules
	goMod, err := modules.findGoMod(ctx, pkgRoot)
	return goMod.rootDir, err
}

type lookupKey struct{ test, mod, work bool }
