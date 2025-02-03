package modulefiles

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/iwahbe/helpmakego/internal/pkg/display"
	"github.com/stretchr/testify/assert"
)

type findTest struct {
	name     string
	files    map[string]string
	expected []string

	includeTestFiles bool
}

func (tc findTest) run(t *testing.T) {
	t.Parallel()
	t.Helper()

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Write files to the temporary directory
	for path, content := range tc.files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		assert.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		assert.NoError(t, err)
	}

	// Run the Find function
	files, err := Find(ctx, tmpDir, tc.includeTestFiles)
	if assert.NoError(t, err) {
		assert.ElementsMatch(t, tc.expected, display.Relative(ctx, tmpDir, files))
	}
}

func TestFindIntegration(t *testing.T) {
	{ // Disable logging for the test
		oldLogLevel := slog.SetLogLoggerLevel(10)
		t.Cleanup(func() { slog.SetLogLoggerLevel(oldLogLevel) })

	}

	tests := []findTest{
		{
			name: "Single Package",
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
		},
		{
			name: "2 Packages",
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
		},
		{
			name: "2 Packages",
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
		},
		{
			name:             "test files",
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
		},

		{
			name: "partial dependency",
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
