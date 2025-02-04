package display

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapePath(t *testing.T) {
	t.Parallel()

	tests := []struct{ input, expected string }{
		{"file.go", "file.go"},
		{"a file.go", "'a file.go'"},
		{`my-"embed".svg`, `'my-"embed".svg'`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			actual := escapePath(context.Background(), tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
