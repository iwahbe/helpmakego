package modulefiles

import (
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
	excludeModFiles  bool
}

func testFind(t *testing.T, args testFindArgs) {
	t.Helper()

	// Only emit warnings
	ctx := log.New(t.Context(), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
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
	files, err := Find(ctx, path.Join(tmpDir, args.runDir), args.includeTestFiles, !args.excludeModFiles)
	if assert.NoError(t, err) {
		assert.ElementsMatch(t, args.expected, display.Relative(ctx, tmpDir, files))
	}
}

func TestFindWithGoWorkspaceEnclosingTwoModules(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "pkg1",
		files: map[string]string{
			"pkg1/go.mod": `module example.com/pkg1

go 1.18
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
			"go.work": `go 1.18

use ./pkg1
use ./pkg2
`,
			"go.work.sum": ``,
		},
		expected: []string{
			"pkg1/go.mod",
			"pkg1/main.go",
			"pkg2/go.mod",
			"pkg2/pkg.go",
			"go.work",
			"go.work.sum",
		},
	})
}

func TestFindWithGoWork(t *testing.T) {
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
			"go.work": `go 1.18

use .
`,
			"go.work.sum": ``,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"go.work",
			"go.work.sum",
		},
	})
}

func TestFindWithNestedGoWork(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "./pkg",
		files: map[string]string{
			"pkg/go.mod": `module example.com/testmod

go 1.18
`,
			"pkg/main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`,
			"go.work": `go 1.18

use ./pkg
`,
			"go.work.sum": ``,
		},
		expected: []string{
			"pkg/go.mod",
			"pkg/main.go",
			"go.work",
			"go.work.sum",
		},
	})
}

func TestFindWithNestedModuleInsideAnother(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "a",
		files: map[string]string{
			"a/go.mod": `module example.com/a

go 1.18

require example.com/b v0.0.0

replace example.com/b => ./b
`,
			"a/main.go": `package main

import (
	"fmt"
	"example.com/b"
)

func main() {
	fmt.Println(b.MessageB())
}
`,
			"a/b/go.mod": `module example.com/b

go 1.18
`,
			"a/b/b.go": `package b

func MessageB() string {
	return "Hello from nested Module B"
}
`,
			"go.work": `go 1.18

use ./a
use ./a/b
`,
			"go.work.sum": ``,
		},
		expected: []string{
			"a/go.mod",
			"a/main.go",
			"a/b/go.mod",
			"a/b/b.go",
			"go.work",
			"go.work.sum",
		},
	})
}

func TestFindNestedModules(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "pkg1",
		files: map[string]string{
			"pkg1/go.mod": `module example.com/pkg1

go 1.18

require example.com/pkg2 v0.0.0

replace example.com/pkg2 => ./pkg2
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
			"pkg1/pkg2/go.mod": `module example.com/pkg2

go 1.18
`,
			"pkg1/pkg2/pkg.go": `package pkg2

func Message() string {
	return "Hello from nested pkg2!"
}
`,
		},
		expected: []string{
			"pkg1/go.mod",
			"pkg1/main.go",
			"pkg1/pkg2/go.mod",
			"pkg1/pkg2/pkg.go",
		},
	})
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

func TestFindSinglePackageNoMod(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		excludeModFiles: true,
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

func TestFindMultipleGoFilesInDirectory(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		files: map[string]string{
			"go.mod": `module example.com/multi

go 1.18
`,
			"main.go": `package main

import (
	"fmt"
	"example.com/multi/utility"
)

func main() {
	fmt.Println(Greet())
	fmt.Println(HelpMessage())

	// Use a function from the local dependency
	fmt.Println(utility.UtilityMessage())
}
`,
			"util.go": `package main

func Greet() string { return "Hello from util!" }
`,
			"helper.go": `package main

func HelpMessage() string { return "This is a help message from helper." }
`,
			"utility/utils.go": `package utility

func UtilityMessage() string { return "This is a message from the utility package." }
`,
			"utility/moreutils.go": `package utility

func MoreUtility() string {r eturn "This is another utility function." }
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"util.go",
			"helper.go",
			"utility/utils.go",
			"utility/moreutils.go",
		},
	})
}

func TestFindExcludeNonGoFiles(t *testing.T) {
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
			"README.md": `# This is the README for example.com/testmod
`,
			"LICENSE": `<LICENSE>
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"pkg/pkg.go",
		},
	})
}

func TestFindWithExternalDependencies(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		files: map[string]string{
			"go.mod": `module example.com/testmod

go 1.18

require github.com/some/external/pkg v1.2.3
`,
			"main.go": `package main

import (
	"fmt"

	"example.com/testmod/localpkg"
)

func main() {
	fmt.Println(localpkg.Message())
}
`,
			"localpkg/pkg.go": `package localpkg

func Message() string {n
	return "Hello from localpkg!"
}
`,
		},
		expected: []string{
			"go.mod",
			"main.go",
			"localpkg/pkg.go",
		},
	})
}
