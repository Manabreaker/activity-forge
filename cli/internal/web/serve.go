package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

func Serve(ctx context.Context, opts Options, port int, open bool, out io.Writer) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("port must be 0–65535")
	}
	s, err := New(opts)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	server := &http.Server{Handler: s.Handler(listener.Addr().String()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	url := "http://" + listener.Addr().String() + "/"
	fmt.Fprintf(out, "\nVisual Preview: %s\nBound to loopback only. Press Ctrl+C to close.\n", url)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if open {
		if err := OpenURL(ctx, url); err != nil {
			fmt.Fprintf(out, "Could not open browser automatically: %v\nOpen the URL above manually.\n", err)
		}
	}
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func OpenURL(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", url).Run()
	}
	if runtime.GOOS == "darwin" {
		return exec.CommandContext(ctx, "open", url).Run()
	}
	return exec.CommandContext(ctx, "xdg-open", url).Run()
}
