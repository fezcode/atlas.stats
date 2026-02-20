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

	if err := ui.Start(); err != nil {
		fmt.Printf("Error starting UI: %v\n", err)
		os.Exit(1)
	}
}
