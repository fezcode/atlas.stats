package main

import (
	"fmt"
	"os"

	"atlas.stats/pkg/ui"
)

func main() {
	if err := ui.Start(); err != nil {
		fmt.Printf("Error starting UI: %v\n", err)
		os.Exit(1)
	}
}
