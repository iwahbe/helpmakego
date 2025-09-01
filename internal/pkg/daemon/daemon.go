package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
)

const serverTimeout time.Duration = time.Second * 5

// Serve a daemon to maintain the cache in the background.
func Serve(ctx context.Context, pkgRoot string) error {
	cache, err := modulefiles.NewCache(ctx, pkgRoot)
	if err != nil {
		return err
	}
	path := socketPath(cache.ModuleRoot())

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}
	defer func() { _ = listener.Close() }()

	setDeadline := func() error {
		if err := listener.(*net.UnixListener).SetDeadline(time.Now().Add(serverTimeout)); err != nil {
			return fmt.Errorf("failed set listener deadline: %w", err)
		}
		return nil
	}

	if err := setDeadline(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	for {
		conn, err := listener.Accept()
		if t, ok := err.(net.Error); ok && t.Timeout() {
			wg.Wait() // Allow ongoing connections to exit
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to accept connection: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = conn.Close() }()
			handle(ctx, cache, conn)
		}()
		if err := setDeadline(); err != nil {
			return err
		}
	}
}

func start(ctx context.Context, moduleRoot string) {
	cmd := exec.CommandContext(context.WithoutCancel(ctx), os.Args[0], "--x-daemon", moduleRoot)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
	if err := cmd.Start(); err != nil {
		log.Warn(ctx, fmt.Sprintf("failed to start daemon: %s", err))
		return
	}
	if err := cmd.Process.Release(); err != nil {
		log.Warn(ctx, fmt.Sprintf("failed to release daemon: %s", err))
	}
}

func socketPath(moduleRoot string) string {
	hash := sha256.Sum256([]byte(moduleRoot))
	encoded := hex.EncodeToString(hash[:])[:32] // Take first 32 chars of hex
	return "/tmp/helpmakego-" + encoded + ".sock"
}

// Find delegates a find call to the running daemon, or it executes the call locally and
// while starting the daemon.
func Find(ctx context.Context, pkgRoot string, includeTests, includeMod, goWork bool) ([]string, error) {
	moduleRoot, err := modulefiles.FindModuleRoot(ctx, pkgRoot)
	if err != nil {
		return nil, err
	}
	socketPath := socketPath(moduleRoot)
	ctx = log.WithAttr(ctx, "socket", socketPath)
	conn, err := net.Dial("unix", socketPath)
	switch {
	case err == nil:
		log.Info(ctx, "connected to existing server")
	case errors.Is(err, syscall.ECONNREFUSED):
		log.Info(ctx, "restarting daemon")
		_ = os.Remove(socketPath)
		fallthrough
	case errors.Is(err, os.ErrNotExist):
		go start(ctx, moduleRoot) // Start the daemon in the background for the next invocation
		log.Info(ctx, "starting daemon for next run")
		return modulefiles.Find(ctx, pkgRoot, includeTests, includeMod, goWork)
	case errors.Is(err, os.ErrPermission):
		log.Warn(ctx, "permission denied to start daemon", log.Attr("error", err.Error()))
		return modulefiles.Find(ctx, pkgRoot, includeTests, includeMod, goWork)
	default:
		return nil, fmt.Errorf("unexpected dial error for find daemon: %w", err)
	}

	enc := json.NewEncoder(conn)
	enc.SetEscapeHTML(false)
	err = enc.Encode(request{
		PathToPackage: pkgRoot,
		IncludeTest:   includeTests,
		IncludeMod:    includeMod,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}
	dec := json.NewDecoder(conn)
	dec.DisallowUnknownFields()
	var resp response
	if err := dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if resp.Error != "" {
		return resp.Files, errors.New(resp.Error)
	}
	return resp.Files, nil
}

func handle(ctx context.Context, cache modulefiles.Cache, conn net.Conn) {

	// Read the request from the client CLI
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	decoder := json.NewDecoder(conn)
	decoder.DisallowUnknownFields()
	var req request
	if err := decoder.Decode(&req); err != nil {
		enc := json.NewEncoder(conn)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(response{
			Error: err.Error(),
		})
		return
	}

	// Execute find from the shared cache
	files, err := cache.Find(ctx, req.PathToPackage, req.IncludeTest, req.IncludeMod, req.GoWork)

	// Write the response
	enc := json.NewEncoder(conn)
	errStr := func(err error) string {
		if err != nil {
			return err.Error()
		}
		return ""
	}
	enc.SetEscapeHTML(false)
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	_ = enc.Encode(response{
		Files: files,
		Error: errStr(err),
	})
}

type request struct {
	PathToPackage string `json:"pathToPackage"`
	IncludeTest   bool   `json:"includeTest"`
	IncludeMod    bool   `json:"includeMod"`
	GoWork        bool   `json:"goWork"`
}

type response struct {
	Files []string
	Error string
}
