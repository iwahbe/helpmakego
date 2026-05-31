package vcs

import (
	"context"
	"fmt"
	"os"
)

// Fake is an in-memory [Repo] for tests. The zero value is not usable; set
// RepoRoot at least.
type Fake struct {
	// RepoRoot is returned by Root.
	RepoRoot string
	// Changed maps a ref to the absolute paths ChangedFiles reports for it.
	Changed map[string][]string
	// Files maps a ref to the file contents ReadFileAtRef reports for it, keyed
	// by repository-relative path.
	Files map[string]map[string][]byte
}

var _ Repo = (*Fake)(nil)

func (f *Fake) Root() string { return f.RepoRoot }

func (f *Fake) ChangedFiles(_ context.Context, ref string) ([]string, error) {
	return f.Changed[ref], nil
}

func (f *Fake) ReadFileAtRef(_ context.Context, ref, path string) ([]byte, error) {
	if content, ok := f.Files[ref][path]; ok {
		return content, nil
	}
	return nil, fmt.Errorf("%s at %s: %w", path, ref, os.ErrNotExist)
}
