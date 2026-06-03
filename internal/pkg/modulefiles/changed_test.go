package modulefiles

import (
	"context"
	"go/build"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iwahbe/helpmakego/internal/pkg/display"
	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"github.com/iwahbe/helpmakego/internal/pkg/vcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type changedTest struct {
	files  map[string]string // path:content of the working tree
	runDir string            // entry point relative to files

	changed  []string          // paths, relative to root, that differ since the ref
	oldFiles map[string]string // repo-relative path:content as of the ref

	includeTestFiles bool
	includeMod       bool
	emitPackages     bool

	expected []string
}

// changedRef is the synthetic revision name the fake repository answers for.
const changedRef = "REF"

func testChanged(t *testing.T, tt changedTest) {
	t.Helper()

	ctx := log.New(t.Context(), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	tmpDir := t.TempDir()
	for path, content := range tt.files {
		full := filepath.Join(tmpDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0644))
	}

	changed := make([]string, len(tt.changed))
	for i, path := range tt.changed {
		changed[i] = filepath.Join(tmpDir, path)
	}
	oldFiles := map[string][]byte{}
	for path, content := range tt.oldFiles {
		oldFiles[path] = []byte(content)
	}
	repo := &vcs.Fake{
		RepoRoot: tmpDir,
		Changed:  map[string][]string{changedRef: changed},
		Files:    map[string]map[string][]byte{changedRef: oldFiles},
	}
	discover := func(context.Context, string) (vcs.Repo, error) { return repo, nil }

	files, err := findWithModules(ctx, filepath.Join(tmpDir, tt.runDir), FindArgs{
		TestPaths:    tt.includeTestFiles,
		ModFiles:     tt.includeMod,
		GoWork:       true,
		EmitPackages: tt.emitPackages,
		ChangedSince: changedRef,
	}, discover, new(modules), &build.Default)
	require.NoError(t, err)
	assert.ElementsMatch(t, tt.expected, display.Relative(ctx, tmpDir, files))
}

// chainFiles is main -> pkgb -> pkga, all in module example.com/m.
func chainFiles() map[string]string {
	return map[string]string{
		"go.mod": `module example.com/m

go 1.18
`,
		"main.go": `package main

import "example.com/m/pkgb"

func main() { _ = pkgb.B() }
`,
		"pkgb/b.go": `package pkgb

import "example.com/m/pkga"

func B() string { return pkga.A() }
`,
		"pkga/a.go": `package pkga

func A() string { return "a" }
`,
	}
}

func TestChangedLeafPropagatesToImporters(t *testing.T) {
	t.Parallel()
	testChanged(t, changedTest{
		files:        chainFiles(),
		emitPackages: true,
		changed:      []string{filepath.Join("pkga", "a.go")},
		expected:     []string{".", "pkgb", "pkga"},
	})
}

func TestChangedRootOnly(t *testing.T) {
	t.Parallel()
	testChanged(t, changedTest{
		files:        chainFiles(),
		emitPackages: true,
		changed:      []string{"main.go"},
		expected:     []string{"."},
	})
}

func TestChangedNothing(t *testing.T) {
	t.Parallel()
	testChanged(t, changedTest{
		files:        chainFiles(),
		emitPackages: true,
		changed:      nil,
		expected:     []string{},
	})
}

func TestChangedFilesModeIncludesModFiles(t *testing.T) {
	t.Parallel()
	testChanged(t, changedTest{
		files:      chainFiles(),
		includeMod: true,
		changed:    []string{filepath.Join("pkga", "a.go")},
		expected: []string{
			"go.mod",
			"main.go",
			filepath.Join("pkgb", "b.go"),
			filepath.Join("pkga", "a.go"),
		},
	})
}

func TestChangedEmbedMarksPackage(t *testing.T) {
	t.Parallel()
	files := chainFiles()
	files["pkga/a.go"] = `package pkga

import _ "embed"

//go:embed data.txt
var data string

func A() string { return data }
`
	files["pkga/data.txt"] = "hello"

	testChanged(t, changedTest{
		files:        files,
		emitPackages: true,
		changed:      []string{filepath.Join("pkga", "data.txt")},
		expected:     []string{".", "pkgb", "pkga"},
	})
}

func TestChangedGoModBumpAffectsImporters(t *testing.T) {
	t.Parallel()
	files := chainFiles()
	files["go.mod"] = `module example.com/m

go 1.18

require github.com/ext/lib v1.1.0
`
	files["pkga/a.go"] = `package pkga

import _ "github.com/ext/lib"

func A() string { return "a" }
`

	testChanged(t, changedTest{
		files:        files,
		emitPackages: true,
		changed:      []string{"go.mod"},
		oldFiles: map[string]string{
			"go.mod": `module example.com/m

go 1.18

require github.com/ext/lib v1.0.0
`,
		},
		expected: []string{".", "pkgb", "pkga"},
	})
}

// TestChangedCrossRepoReplace exercises a local replace whose target lives in a
// separate repository. The change in that repository is found by diffing it from
// the tag the entry go.mod pinned at the ref.
func TestChangedCrossRepoReplace(t *testing.T) {
	t.Parallel()

	ctx := log.New(t.Context(), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	tmpDir := t.TempDir()
	files := map[string]string{
		"app/go.mod": `module example.com/app

go 1.18

require example.com/lib v1.0.0

replace example.com/lib => ../lib
`,
		"app/main.go": `package main

import "example.com/lib/widget"

func main() { _ = widget.W() }
`,
		"lib/go.mod": `module example.com/lib

go 1.18
`,
		"lib/widget/widget.go": `package widget

import "example.com/lib/helper"

func W() string { return helper.H() }
`,
		"lib/helper/helper.go": `package helper

func H() string { return "h" }
`,
	}
	for path, content := range files {
		full := filepath.Join(tmpDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0644))
	}

	appRoot := filepath.Join(tmpDir, "app")
	libRoot := filepath.Join(tmpDir, "lib")

	app := &vcs.Fake{
		RepoRoot: appRoot,
		// Nothing changed in the app repository itself.
		Changed: map[string][]string{changedRef: nil},
		// The app go.mod pinned example.com/lib v1.0.0 at the ref.
		Files: map[string]map[string][]byte{changedRef: {"go.mod": []byte(files["app/go.mod"])}},
	}
	lib := &vcs.Fake{
		RepoRoot: libRoot,
		// widget changed in the lib repository since its v1.0.0 tag.
		Changed: map[string][]string{
			"v1.0.0": {filepath.Join(libRoot, "widget", "widget.go")},
		},
	}
	discover := func(_ context.Context, dir string) (vcs.Repo, error) {
		if dir == libRoot || strings.HasPrefix(dir, libRoot+string(os.PathSeparator)) {
			return lib, nil
		}
		return app, nil
	}

	result, err := findWithModules(ctx, appRoot, FindArgs{
		GoWork:       true,
		EmitPackages: true,
		ChangedSince: changedRef,
	}, discover, new(modules), &build.Default)
	require.NoError(t, err)

	// widget changed, so widget and its importer app are emitted. helper is a
	// dependency of widget, not an importer, so it is excluded.
	assert.ElementsMatch(t, []string{"app", filepath.Join("lib", "widget")},
		display.Relative(ctx, tmpDir, result))
}

func TestChangedGoModUnchangedExternal(t *testing.T) {
	t.Parallel()
	files := chainFiles()
	files["go.mod"] = `module example.com/m

go 1.18

require github.com/ext/lib v1.0.0
`
	files["pkga/a.go"] = `package pkga

import _ "github.com/ext/lib"

func A() string { return "a" }
`

	// The go.mod is identical at the ref, so nothing is affected.
	testChanged(t, changedTest{
		files:        files,
		emitPackages: true,
		changed:      nil,
		oldFiles: map[string]string{
			"go.mod": files["go.mod"],
		},
		expected: []string{},
	})
}
