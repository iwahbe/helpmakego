package deamon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/iwahbe/helpmakego/internal/pkg/log"
	"github.com/iwahbe/helpmakego/internal/pkg/modulefiles"
)

// Serve a deamon to maintain the cache in the background.
//
// It lives for 5 seconds.
func Serve(ctx context.Context, pkgRoot string) error {
	cache, err := modulefiles.NewCache(ctx, pkgRoot)
	if err != nil {
		return err
	}
	path := socketPath(cache.ModuleRoot())
	err = os.Remove(path)
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	listner, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}
	defer func() { _ = listner.Close() }()

	// setDeadline gives a 5 second grace period for waiting for the next connection.
	setDeadline := func() error {
		if err := listner.(*net.UnixListener).SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return fmt.Errorf("failed set listener deadline: %w", err)
		}
		return nil
	}

	if err := setDeadline(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	for {
		conn, err := listner.Accept()
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			wg.Wait() // Allow ongoing connections to exit
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to accept connection: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle(ctx, cache, conn)
		}()
		if err := setDeadline(); err != nil {
			return err
		}
	}
}

func start(ctx context.Context, moduleRoot string) {
	cmd := exec.CommandContext(context.WithoutCancel(ctx), os.Args[0], "--x-deamon", moduleRoot)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
	if err := cmd.Start(); err != nil {
		log.Warn(ctx, fmt.Sprintf("failed to start deamon: %s", err))
		return
	}
	if err := cmd.Process.Release(); err != nil {
		log.Warn(ctx, fmt.Sprintf("failed to release deamon: %s", err))
	}
}

func socketPath(moduleRoot string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(moduleRoot))
	return "/tmp/helpmakego-" + encoded + ".sock"
}

// Find delegates a find call to the running deamon, or it executes the call locally and
// while starting the deamon.
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
	case strings.Contains(err.Error(), "connection refused"):
		log.Info(ctx, "restarting deamon at %s", socketPath)
		_ = os.Remove(socketPath)
		fallthrough
	case errors.Is(err, os.ErrNotExist):
		go start(ctx, moduleRoot) // Start the deamon in the background for the next invocation
		log.Info(ctx, "starting deamon for next run")
		return modulefiles.Find(ctx, pkgRoot, includeTests, includeMod, goWork)
	case errors.Is(err, os.ErrPermission):
		log.Warn(ctx, "permission denied to start deamon", log.Attr("error", err.Error()))
		return modulefiles.Find(ctx, pkgRoot, includeTests, includeMod, goWork)
	default:
		return nil, fmt.Errorf("unexpected dial error for find deamon: %w", err)
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
