package daemon

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
)

func TestDaemon(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var logOut bytes.Buffer
	ctx = log.New(ctx, slog.New(slog.NewTextHandler(&logOut, nil)))
	// Poor man's check that we actually connected with the underlying server.
	defer func() { assert.Contains(t, logOut.String(), "connected to existing server") }()

	// Create artificial Go module structure
	tmpDir := t.TempDir()
	setupArtificialGoModule(t, tmpDir)

	serverDone := make(chan error, 1)
	go func() {
		// Start daemon.Serve in background goroutine with shorter timeout context
		serverCtx, serverCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer serverCancel()

		err := Serve(serverCtx, tmpDir)
		serverDone <- err
	}()

	// Wait for daemon to start listening by polling for socket file
	cache, err := modulefiles.NewCache(ctx, tmpDir)
	require.NoError(t, err)
	socketPath := socketPath(cache.ModuleRoot())

	var socketExists bool
	for range 50 { // Wait up to 5 seconds
		if _, err := os.Stat(socketPath); err == nil {
			socketExists = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(t, socketExists, "daemon socket was not created: %s", socketPath)

	// Test daemon.Find - should connect to running daemon
	files, err := Find(ctx, tmpDir, false, true, true)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	assert.Equal(t, []string{
		tmpDir + "/go.mod",
		tmpDir + "/main.go",
	}, files)

	// Wait for daemon to exit via its own timeout (2 seconds + margin)
	select {
	case err := <-serverDone:
		require.NoError(t, err)
	case <-time.After(6 * time.Second):
		t.Fatal("daemon didn't shut down cleanly")
	}
}

func setupArtificialGoModule(t *testing.T, dir string) {
	// Create go.mod
	gomod := `module test.example/foo
go 1.24
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644))

	// Create main.go
	main := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0644))
}
