package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(t *testing.M) {
	// Make sure that our integration tests don't change depending on the calling
	// environment.
	err := os.Setenv("HELPMAKEGO_EXPERIMENT_DAEMON", "false")
	if err != nil {
		panic(err)
	}
	os.Exit(t.Run())
}

func TestIntegrationMinimal(t *testing.T) {
	t.Parallel()

	modRoot, err := filepath.Abs("testdata/minimal")
	require.NoError(t, err)

	cmd := exec.CommandContext(t.Context(), "go", "run", ".",
		modRoot, "--test", "--mod", "--abs", "--json")
	output, err := cmd.Output()
	require.NoError(t, err)

	var paths []string
	require.NoError(t, json.Unmarshal(output, &paths))

	assert.ElementsMatch(t, []string{
		filepath.Join(modRoot, "go.mod"),
		filepath.Join(modRoot, "main.go"),
	}, paths)
}

func TestIntegrationWork(t *testing.T) {
	t.Parallel()

	modRoot, err := filepath.Abs("testdata/workspace/b")
	require.NoError(t, err)

	cmd := exec.CommandContext(t.Context(), "go", "run", ".",
		modRoot, "--test", "--mod", "--abs", "--json")
	output, err := cmd.Output()
	require.NoError(t, err)

	var paths []string
	require.NoError(t, json.Unmarshal(output, &paths))

	assert.ElementsMatch(t, []string{
		filepath.Join(modRoot, "go.mod"),
		filepath.Join(modRoot, "main.go"),
		filepath.Join(modRoot, "..", "go.work"),
		filepath.Join(modRoot, "..", "a", "go.mod"),
		filepath.Join(modRoot, "..", "a", "pkg.go"),
	}, paths)
}
