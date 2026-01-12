package main

import (
	"log"
	"os"

	"github.com/mindmorass/yippity-clippity/internal/app"
)

// Version is set at build time
var Version = "dev"

func main() {
	// Set up logging
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Create and run application
	application, err := app.New(Version)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
		os.Exit(1)
	}
}
