package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
)

const shutdownTimeout = 30 * time.Second

func main() {
	svc := lifecycle.NewService(baseagent.NoopTriager{})

	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		log.Fatalf("register openclaw manifest: %v", err)
	}

	fmt.Println("agentd scaffold booted")
	fmt.Println("catalog:")
	for _, entry := range catalog.DefaultEntries() {
		fmt.Printf("- %s (%s): %s\n", entry.Name, entry.ID, entry.Status)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Block until signal received
	<-ctx.Done()
	fmt.Println("shutdown signal received, stopping agents...")

	if err := shutdownAgents(svc, shutdownTimeout); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	fmt.Println("agentd stopped gracefully")
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
			if err := svc.Stop(agent.ID); err != nil {
				fmt.Fprintf(os.Stderr, "failed to stop agent %s: %v\n", agent.ID, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}
