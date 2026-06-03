package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRepo initializes a git repository in a temporary directory with a single
// committed file and returns its root.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("could not resolve git: %s", err.Error())
	}

	// git rev-parse resolves symlinks, so resolve the temp dir up front to keep
	// the paths it reports comparable with the ones the test constructs.
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	gitDo := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	gitDo("init")
	gitDo("config", "user.email", "test@example.com")
	gitDo("config", "user.name", "Test")
	writeFile(t, root, "committed.txt", "original")
	gitDo("add", "committed.txt")
	gitDo("commit", "-m", "initial")

	return root
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0644))
}

func TestGitChangedFiles(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)

	repo, err := Discover(t.Context(), root)
	require.NoError(t, err)
	assert.Equal(t, root, repo.Root())

	// A clean tree at HEAD reports nothing.
	changed, err := repo.ChangedFiles(t.Context(), "HEAD")
	require.NoError(t, err)
	assert.Empty(t, changed)

	// Modify a tracked file and add an untracked one.
	writeFile(t, root, "committed.txt", "modified")
	writeFile(t, root, "pkg/new.txt", "new")

	changed, err = repo.ChangedFiles(t.Context(), "HEAD")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		filepath.Join(root, "committed.txt"),
		filepath.Join(root, "pkg", "new.txt"),
	}, changed)
}

func TestGitChangedFilesNetRevert(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)

	repo, err := Discover(t.Context(), root)
	require.NoError(t, err)

	// Reverting a tracked file to its HEAD contents leaves no net change.
	writeFile(t, root, "committed.txt", "original")
	changed, err := repo.ChangedFiles(t.Context(), "HEAD")
	require.NoError(t, err)
	assert.Empty(t, changed)
}

func TestGitReadFileAtRef(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)

	repo, err := Discover(t.Context(), root)
	require.NoError(t, err)

	// The working tree differs from HEAD, but the ref still sees the original.
	writeFile(t, root, "committed.txt", "modified")
	content, err := repo.ReadFileAtRef(t.Context(), "HEAD", "committed.txt")
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
}

func TestGitReadFileAtRefMissing(t *testing.T) {
	t.Parallel()
	root := gitRepo(t)

	repo, err := Discover(t.Context(), root)
	require.NoError(t, err)

	_, err = repo.ReadFileAtRef(t.Context(), "HEAD", "absent.txt")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDiscoverNotARepository(t *testing.T) {
	t.Parallel()
	_, err := Discover(t.Context(), t.TempDir())
	assert.ErrorIs(t, err, ErrNotARepository)
}
