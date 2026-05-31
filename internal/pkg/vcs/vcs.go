// Package vcs abstracts over a version control system so that helpmakego can ask
// what has changed since a given revision.
//
// Go modules may live in repositories managed by Git, Mercurial, and others, so
// callers depend on the [Repo] interface rather than any single tool. The
// interface is small enough to mock; see [Fake] for an in-memory implementation
// suitable for tests.
package vcs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// Repo is a version control repository rooted at a single directory.
type Repo interface {
	// Root returns the absolute path to the root of the repository.
	Root() string

	// ChangedFiles returns the absolute paths of every file that differs between
	// ref and the current working tree.
	//
	// This is the net difference and includes files changed by intervening
	// commits, uncommitted changes (both staged and unstaged), and untracked
	// files. A file changed in an intervening commit but reverted to its ref
	// contents is not reported.
	ChangedFiles(ctx context.Context, ref string) ([]string, error)

	// ReadFileAtRef returns the contents of the file at path, relative to the
	// repository root, as of ref.
	//
	// It returns an error wrapping [os.ErrNotExist] if the file did not exist at
	// ref.
	ReadFileAtRef(ctx context.Context, ref, path string) ([]byte, error)
}

// ErrNotARepository is returned by [Discover] when dir is not contained in any
// supported version control repository.
var ErrNotARepository = errors.New("not in a version control repository")

// ErrUnsupportedVCS is returned by [Discover] when dir is contained in a
// repository whose version control system helpmakego does not yet support.
var ErrUnsupportedVCS = errors.New("unsupported version control system")

// Discover finds the version control repository that contains dir.
//
// It walks up from dir looking for a known repository marker. It returns
// [ErrNotARepository] if no marker is found and [ErrUnsupportedVCS] if a marker
// for an unsupported system is found.
func Discover(ctx context.Context, dir string) (Repo, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	for {
		switch marker, err := markerAt(dir); {
		case err != nil:
			return nil, err
		case marker == ".git":
			return newGit(ctx, dir)
		case marker != "":
			return nil, ErrUnsupportedVCS
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ErrNotARepository
		}
		dir = parent
	}
}

// markerAt returns the name of the version control marker present in dir, or the
// empty string if none is present.
func markerAt(dir string) (string, error) {
	// Markers for systems we do not yet support are still detected so that
	// Discover can report ErrUnsupportedVCS rather than walking past the
	// enclosing repository.
	for _, marker := range []string{".git", ".hg", ".svn", ".bzr", ".fossil"} {
		switch _, err := os.Stat(filepath.Join(dir, marker)); {
		case err == nil:
			return marker, nil
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return "", err
		}
	}
	return "", nil
}
