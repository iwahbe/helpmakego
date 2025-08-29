package modulefiles

import "context"

type Cache struct{}

func NewCache(moduleRoot string) Cache {
	panic("TODO")
}

func (c Cache) Find(ctx context.Context, pkg string, testPaths, modFiles bool) ([]string, error) {
	panic("TODO")
}
