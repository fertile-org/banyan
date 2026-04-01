package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/cmd/banyan-cli/webdist"
)

// runWebDashboard starts a local web server that serves the embedded React
// dashboard and reverse-proxies API calls to the engine's connect-go endpoint.
func runWebDashboard(port, engineGRPCAddr string) error {
	webFS := webdist.FS()
	if webFS == nil {
		return fmt.Errorf("web dashboard not bundled in this build. Rebuild with: make build")
	}

	// The engine's connect-go API runs on a separate port (default 9091).
	// Derive it from the gRPC address: same host, port 9091.
	apiURL, err := resolveAPIURL(engineGRPCAddr)
	if err != nil {
		return fmt.Errorf("cannot resolve engine API URL: %w", err)
	}

	mux := http.NewServeMux()

	// Reverse proxy: /banyan.v1.EngineService/* → engine connect-go API
	proxy := httputil.NewSingleHostReverseProxy(apiURL)
	mux.Handle("/banyan.v1.EngineService/", proxy)

	// Static files: everything else → embedded React app (with SPA fallback)
	mux.Handle("/", spaHandler(webFS))

	addr := "127.0.0.1:" + port
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	dashURL := fmt.Sprintf("http://localhost:%s", port)
	fmt.Printf("\n  Banyan Dashboard\n\n")
	fmt.Printf("  Local:   %s\n", dashURL)
	fmt.Printf("  Engine:  %s\n", apiURL.String())
	fmt.Printf("\n  Press Ctrl+C to stop\n\n")

	// Try to open browser
	_ = openBrowser(dashURL)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("web server error: %w", err)
	}
	return nil
}

// spaHandler serves static files from the embedded FS, falling back to
// index.html for client-side routes (React Router).
func spaHandler(webFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(webFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(webFS, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// File not found → serve index.html (SPA fallback)
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// resolveAPIURL derives the connect-go API URL from the gRPC address.
// The connect-go API runs on port 9091 on the same host as gRPC.
func resolveAPIURL(grpcAddr string) (*url.URL, error) {
	host, _, err := net.SplitHostPort(grpcAddr)
	if err != nil {
		// Might be just a host without port
		host = grpcAddr
	}
	return url.Parse(fmt.Sprintf("http://%s:9091", host))
}

// openBrowser tries to open the URL in the default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return nil
	}
}
