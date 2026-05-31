package vcs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// git is a [Repo] backed by the git command line.
type git struct{ root string }

// newGit constructs a git [Repo] for the repository containing dir.
//
// It resolves dir to the repository's top level so that paths reported by git
// are relative to root.
func newGit(ctx context.Context, dir string) (Repo, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	return git{root: filepath.Clean(strings.TrimSpace(string(out)))}, nil
}

func (g git) Root() string { return g.root }

func (g git) ChangedFiles(ctx context.Context, ref string) ([]string, error) {
	// The two commands are independent, so run them concurrently.
	var diff, untracked []byte
	var diffErr, untrackedErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// diff reports the net difference between ref and the working tree, covering
		// intervening commits as well as staged and unstaged changes.
		diff, diffErr = run(ctx, g.root, "diff", "-z", "--name-only", ref, "--")
	}()
	go func() {
		defer wg.Done()
		// ls-files adds untracked files, which diff does not report.
		untracked, untrackedErr = run(ctx, g.root, "ls-files", "-z", "--others", "--exclude-standard")
	}()
	wg.Wait()
	if err := errors.Join(diffErr, untrackedErr); err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	var files []string
	for _, rel := range append(splitNUL(diff), splitNUL(untracked)...) {
		abs := filepath.Join(g.root, rel)
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		files = append(files, abs)
	}
	return files, nil
}

func (g git) ReadFileAtRef(ctx context.Context, ref, path string) ([]byte, error) {
	out, err := run(ctx, g.root, "show", ref+":"+filepath.ToSlash(path))
	if err != nil {
		// git reports a missing path on stderr, which run folds into err.
		if strings.Contains(err.Error(), "does not exist") ||
			strings.Contains(err.Error(), "exists on disk, but not in") {
			return nil, fmt.Errorf("%s at %s: %w", path, ref, os.ErrNotExist)
		}
		return nil, err
	}
	return out, nil
}

func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func splitNUL(b []byte) []string {
	b = bytes.TrimRight(b, "\x00")
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}
