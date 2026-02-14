package main

import (
	"fmt"
	"log"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
)

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
}
