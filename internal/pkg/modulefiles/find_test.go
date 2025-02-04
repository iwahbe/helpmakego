package modulefiles

import (
	"context"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/iwahbe/helpmakego/internal/pkg/display"
	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"github.com/stretchr/testify/assert"
)

type testFindArgs struct {
	files map[string]string // A path:content map of files for the test

	expected []string // Files that Find is expected to surface.
	runDir   string   // The path to the entry point in files.

	includeTestFiles bool
}

func testFind(t *testing.T, args testFindArgs) {
	t.Helper()

	// Only emit warnings
	ctx := log.New(context.Background(), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	tmpDir := t.TempDir()

	// Write files to the temporary directory
	for path, content := range args.files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		assert.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		assert.NoError(t, err)
	}

	// Run the Find function
	files, err := Find(ctx, path.Join(tmpDir, args.runDir), args.includeTestFiles)
	if assert.NoError(t, err) {
		assert.ElementsMatch(t, args.expected, display.Relative(ctx, tmpDir, files))
	}
}

func TestFindSinglePackage(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		files: map[string]string{
			"go.mod": `module example.com/testmod

go 1.18
`,
			"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
		},
	})
}

func TestFindTwoPackages(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		files: map[string]string{
			"go.mod": `module example.com/testmod

go 1.18
`,
			"main.go": `package main

import (
	"fmt"
	"example.com/testmod/pkg"
)

func main() {
	fmt.Println(pkg.Message())
}
`,
			"pkg/pkg.go": `package pkg

func Message() string {
	return "Hello from pkg!"
}
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"pkg/pkg.go",
		},
	})
}

func TestFindTestsExcluded(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		includeTestFiles: false,
		files: map[string]string{
			"go.mod": `module example.com/testmod

go 1.18
`,
			"main.go": `package main

import (
	"fmt"
	"example.com/testmod/pkg"
)

func main() {
	fmt.Println(pkg.Message())
}
`,
			"pkg/pkg.go": `package pkg

func Message() string {
	return "Hello from pkg!"
}
`,
			"pkg/pkg_test.go": `package pkg

import "testing"

func TestMessage(t *testing.T) string {
	if Message() != "Hello from pkg!" {
		t.Fail()
	}
}
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"pkg/pkg.go",
		},
	})
}

func TestFindTestsIncluded(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		includeTestFiles: true,
		files: map[string]string{
			"go.mod": `module example.com/testmod

go 1.18
`,
			"main.go": `package main

import (
	"fmt"
	"example.com/testmod/pkg"
)

func main() {
	fmt.Println(pkg.Message())
}
`,
			"pkg/pkg.go": `package pkg

func Message() string {
	return "Hello from pkg!"
}
`,
			"pkg/pkg_test.go": `package pkg

import "testing"

func TestMessage(t *testing.T) string {
	if Message() != "Hello from pkg!" {
		t.Fail()
	}
}
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"pkg/pkg.go",
			"pkg/pkg_test.go",
		},
	})
}

func TestFindPartialDependency(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		files: map[string]string{
			"go.mod": `module example.com/testmod

go 1.18
`,
			"main.go": `package main

import (
	"fmt"
	"example.com/testmod/pkg1"
)

func main() {
	fmt.Println(pkg.Message())
}
`,
			"pkg1/pkg.go": `package pkg1

func Message() string {
	return "Hello from pkg!"
}
`,
			"pkg2/pkg.go": `package pkg2

func Message() string {
	return "Hello from pkg!"
}
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"pkg1/pkg.go",
		},
	})
}

func TestFindSideBySideReplace(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "pkg1",
		files: map[string]string{
			"pkg1/go.mod": `module example.com/pkg1

go 1.18

require example.com/pkg2 v0.0.0

replace example.com/pkg2 => ../pkg2
`,
			"pkg1/main.go": `package main

import (
	"fmt"
	"example.com/pkg2"
)

func main() {
	fmt.Println(pkg2.Message())
}
`,
			"pkg2/go.mod": `module example.com/pkg2

go 1.18
`,
			"pkg2/pkg.go": `package pkg2

func Message() string {
	return "Hello from pkg2!"
}
`,
		},
		expected: []string{
			"pkg1/go.mod",
			"pkg1/main.go",
			"pkg2/go.mod",
			"pkg2/pkg.go",
		},
	})
}
