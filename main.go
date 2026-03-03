package main

import (
	"fmt"
	"os"

	"atlas.stats/pkg/ui"
)

var Version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Printf("atlas.stats v%s\n", Version)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help") {
		fmt.Println("Atlas Stats - Real-time system monitoring (CPU, RAM, Disk, Network).")
		fmt.Println("\nUsage:")
		fmt.Println("  atlas.stats        Start the monitor")
		fmt.Println("  atlas.stats -v     Show version")
		fmt.Println("  atlas.stats -h     Show this help")
		return
	}

	if err := ui.Start(); err != nil {
		fmt.Printf("Error starting UI: %v\n", err)
		os.Exit(1)
	}
}
