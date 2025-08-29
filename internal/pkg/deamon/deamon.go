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
func Serve(ctx context.Context, moduleRoot string) error {
	path := socketPath(moduleRoot)
	err := os.Remove(path)
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	listner, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}
	defer listner.Close()

	// We should clean up the deamon after no new requests for 5 seconds.
	if err := listner.(*net.UnixListener).SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("failed set listener deadline: %w", err)
	}

	cache := modulefiles.NewCache(moduleRoot)

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
	}
}

func start(ctx context.Context, moduleRoot string) {
	cmd := exec.CommandContext(context.WithoutCancel(ctx), os.Args[0], "deamon")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Noctty: true,
	}
	if err := cmd.Start(); err != nil {
		log.Warn(ctx, "failed to start deamon: %s", err)
		return
	}
	if err := cmd.Process.Release(); err != nil {
		log.Warn(ctx, "failed to release deamon: %s", err)
	}
}

func socketPath(moduleRoot string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(moduleRoot))
	return "/tmp/helpmakego-" + encoded + ".sock"
}

func Find(ctx context.Context, pkgRoot string, includeTests, includeMod bool) ([]string, error) {
	moduleRoot, err := modulefiles.FindModuleRoot(pkgRoot)
	if err != nil {
		return nil, err
	}
	socketPath := socketPath(moduleRoot)
	conn, err := net.Dial("unix", socketPath)
	switch {
	case err == nil:
	case strings.Contains(err.Error(), "connection refused"):
		log.Info(ctx, "restarting deamon at %s", socketPath)
		os.Remove(socketPath)
		fallthrough
	case errors.Is(err, os.ErrNotExist):
		go start(ctx, moduleRoot) // Start the deamon in the background for the next invocation
		log.Info(ctx, "starting deamon for next run at %s", socketPath)
		return modulefiles.Find(ctx, pkgRoot, includeTests, includeMod)
	case errors.Is(err, os.ErrPermission):
		log.Warn(ctx, "permission denied to start deamon: %s", err.Error())
		return modulefiles.Find(ctx, pkgRoot, includeTests, includeMod)
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
	files, err := cache.Find(ctx, req.PathToPackage, req.IncludeTest, req.IncludeMod)

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
	IncludeTest   bool   `json:"includeTest,omitzero"`
	IncludeMod    bool   `json:"includeMod,omitzero"`
}

type response struct {
	Files []string
	Error string
}
