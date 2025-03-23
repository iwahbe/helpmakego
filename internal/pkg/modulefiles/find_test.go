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
	"example.com/pkg2"
)

func main() {
	pkg2.Message()
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

func main() {}
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

func main() {}
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
	"example.com/b"
)

func main() {
	b.MessageB()
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
	"example.com/pkg2"
)

func main() {
	pkg2.Message()
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

func main() {}
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

func main() {}
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
	"example.com/testmod/pkg"
)

func main() { pkg.Message() }
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
	"example.com/testmod/pkg"
)

func main() { pkg.Message() }
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
	"example.com/testmod/pkg"
)

func main() { pkg.Message() }
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
	"example.com/testmod/pkg1"
)

func main() { pkg.Message() }
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
	"example.com/pkg2"
)

func main() { pkg2.Message() }
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
	"example.com/multi/utility"
)

func main() {
	Greet()
	HelpMessage()
	utility.UtilityMessage()
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
	"example.com/testmod/pkg"
)

func main() { pkg.Message() }
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
	"example.com/testmod/localpkg"
)

func main() { localpkg.Message() }
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

func TestNestedGoWorkDependency(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "module_a",
		files: map[string]string{
			"module_a/go.mod": `module example.com/module_a

go 1.18

require example.com/module_b v0.0.0

`,
			"module_a/main.go": `package main

import (
	"example.com/module_b"
)

func main() { module_b.BMessage() }
`,
			"module_b/go.mod": `module example.com/module_b

go 1.18

require example.com/module_c v0.0.0

`,
			"module_b/b.go": `package module_b

import "example.com/module_c"

func BMessage() string {
	return "From B: " + module_c.CMessage()
}
`,
			"module_c/go.mod": `module example.com/module_c

go 1.18
`,
			"module_c/c.go": `package module_c

func CMessage() string {
	return "Greetings from module C"
}
`,
			"go.work": `go 1.18

use ./module_a
use ./module_b
use ./module_c
`,
			"go.work.sum": ``,
		},
		expected: []string{
			"module_a/go.mod",
			"module_a/main.go",
			"module_b/go.mod",
			"module_b/b.go",
			"module_c/go.mod",
			"module_c/c.go",
			"go.work",
			"go.work.sum",
		},
	})
}

func TestSimilarReplaces(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "pkg1",
		files: map[string]string{
			"pkg1/go.mod": `module example.com/pkg1

go 1.18

require example.com/pkg2 v0.0.0

replace example.com/pkg2 => ../pkg2
replace example.com/pkg2nested => ../pkg2nested
`,
			"pkg1/main.go": `package main

import (
	"example.com/pkg2"
	"example.com/pkg2nested"
)

func main() { pkg2.Message(); pkg2nested.Message() }
`,
			"pkg2/go.mod": `module example.com/pkg2

go 1.18
`,
			"pkg2/pkg.go": `package pkg2

func Message() string {}
`,
			"pkg2nested/go.mod": `module example.com/pkg2

go 1.18
`,
			"pkg2nested/pkg.go": `package pkg2nested

func Message() string {}
`,
		},
		expected: []string{
			"pkg1/go.mod",
			"pkg1/main.go",
			"pkg2/go.mod",
			"pkg2/pkg.go",
			"pkg2nested/go.mod",
			"pkg2nested/pkg.go",
		},
	})
}

func TestSimilarModules(t *testing.T) {
	t.Parallel()
	testFind(t, testFindArgs{
		runDir: "pkg1",
		files: map[string]string{
			"pkg1/go.mod": `module example.com/pkg1

go 1.18

require example.com/pkg1neested v0.0.0

`,
			"pkg1/main.go": `package main

import (
	"example.com/pkg1nested"
)

func main() { pkg1nested.Message(); pkg2nested.Message() }
`,
		},
		expected: []string{
			"pkg1/go.mod",
			"pkg1/main.go",
		},
	})
}

func TestModuleCovers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		_import, from, expected string
		covers                  bool
	}{
		{
			_import: "k8s.io/apimachinery/pkg/util/net", from: "k8s.io/api",
			covers: false,
		},
		{
			_import: "k8s.io/apimachinery/pkg/util/net", from: "k8s.io/apimachinery",
			covers: true, expected: "pkg/util/net",
		},
		{
			_import: "github.com/example/foo", from: "github.com/example/foo/bar",
			covers: false,
		},
		{
			_import: "github.com/example/foo/bar", from: "github.com/example/foo",
			covers: true, expected: "bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt._import+" "+tt.from, func(t *testing.T) {
			t.Parallel()

			actual, covers := moduleCovers(tt._import, tt.from)
			if assert.Equal(t, tt.covers, covers) {
				assert.Equal(t, tt.expected, actual)
			}
		})
	}
}
