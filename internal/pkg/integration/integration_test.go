package integration_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationMinimal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	modRoot, err := filepath.Abs("testdata/minimal")
	require.NoError(t, err)

	paths, err := modulefiles.Find(ctx, modRoot, true /* includeTest */, true /* includeMod */)

	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{
		filepath.Join(modRoot, "go.mod"),
		filepath.Join(modRoot, "main.go"),
	}, paths)
}

func TestIntegrationWork(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	modRoot, err := filepath.Abs("testdata/workspace/b")
	require.NoError(t, err)

	paths, err := modulefiles.Find(ctx, modRoot, true /* includeTest */, true /* includeMod */)

	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{
		filepath.Join(modRoot, "go.mod"),
		filepath.Join(modRoot, "main.go"),
		filepath.Join(modRoot, "..", "go.work"),
		filepath.Join(modRoot, "..", "a", "go.mod"),
		filepath.Join(modRoot, "..", "a", "pkg.go"),
	}, paths)
}
