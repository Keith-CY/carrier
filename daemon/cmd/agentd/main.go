package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"carrier/daemon/internal/api"
	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/config"
	"carrier/daemon/internal/health"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/logging"
)

const shutdownTimeout = 30 * time.Second

func main() {
	logger := logging.Init()
	logger.Info("initializing agentd")

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	var opts []lifecycle.Option

	window, err := cfg.CrashWindowDuration()
	if err != nil {
		log.Fatalf("invalid crash window %q: %v", cfg.Lifecycle.CrashWindow, err)
	}
	cooldown, err := cfg.CrashCooldownDuration()
	if err != nil {
		log.Fatalf("invalid crash cooldown %q: %v", cfg.Lifecycle.CrashCooldown, err)
	}
	opts = append(opts, lifecycle.WithCrashLoopConfig(cfg.Lifecycle.CrashThreshold, window, cooldown))

	svc := lifecycle.NewService(baseagent.NoopTriager{}, opts...)

	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		log.Fatalf("register openclaw manifest: %v", err)
	}

	logger.Info("agentd scaffold booted")
	fmt.Printf("agentd scaffold booted (listen=%s:%d log=%s/%s)\n",
		cfg.Server.Host, cfg.Server.Port, cfg.Log.Level, cfg.Log.Format)
	fmt.Println("catalog:")
	for _, entry := range catalog.DefaultEntries() {
		fmt.Printf("- %s (%s): %s\n", entry.Name, entry.ID, entry.Status)
	}

	// Validate security: refuse non-loopback binding without an API token.
	if cfg.Server.APIToken == "" && !isLoopback(cfg.Server.Host) {
		log.Fatalf("CARRIER_SERVER_API_TOKEN must be set when listening on non-loopback address %q", cfg.Server.Host)
	}
	if cfg.Server.APIToken == "" {
		// Force loopback-only binding when no token is configured.
		cfg.Server.Host = "127.0.0.1"
		logger.Info("no API token configured; forcing loopback-only bind (127.0.0.1)")
	}

	healthServer := health.NewServer(svc)
	healthServer.SetReady(false)

	handler := buildHTTPHandler(svc, healthServer)
	if cfg.Server.APIToken != "" {
		handler = bearerAuthMiddleware(cfg.Server.APIToken, handler)
	}

	listenAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: handler,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()
	healthServer.SetReady(true)
	logger.Info("agentd HTTP server started")
	fmt.Printf("agentd HTTP server listening on %s\n", listenAddr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	select {
	case <-ctx.Done():
	case err := <-serverErrCh:
		log.Fatalf("http server failed: %v", err)
	}

	healthServer.SetReady(false)
	fmt.Println("shutdown signal received, stopping agents...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "http shutdown error: %v\n", err)
	}

	if err := shutdownAgents(svc, shutdownTimeout); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	fmt.Println("agentd stopped gracefully")
}

func buildHTTPHandler(svc *lifecycle.Service, healthServer *health.Server) http.Handler {
	root := http.NewServeMux()
	healthMux := healthServer.Handler()
	root.Handle("/healthz", healthMux)
	root.Handle("/readyz", healthMux)

	apiServer := api.NewServer(svc)
	root.Handle("/api/v1/", apiServer.Handler())
	return root
}

func shutdownAgents(svc *lifecycle.Service, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- stopAllAgents(svc)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "shutdown timed out after", timeout)
		return ctx.Err()
	}
}

func stopAllAgents(svc *lifecycle.Service) error {
	agents := svc.ListAgents()
	var firstErr error
	for _, agent := range agents {
		if agent.Runtime == lifecycle.RuntimeStateRunning {
			if err := svc.Stop(context.Background(), agent.ID); err != nil {
				fmt.Fprintf(os.Stderr, "failed to stop agent %s: %v\n", agent.ID, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

// bearerAuthMiddleware requires a valid Bearer token for /api/ routes.
// Health-check endpoints (/healthz, /readyz) are exempt.
func bearerAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+token {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopback returns true when the host string resolves to a loopback address.
func isLoopback(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
