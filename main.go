package main

import (
	"embed"
	"log"
	"os"
	"runtime"
	"ssh-connect/internal/app"
	"ssh-connect/internal/desktop/plugins"
	"ssh-connect/internal/service"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) == 2 && os.Args[1] == plugins.WorkerArgument {
		if err := plugins.ServeWorker(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	background := runtime.GOOS == "darwin" && service.LocalTerminalAvailable() && len(os.Args) >= 2 && os.Args[1] == "--agent-background"
	if background && len(os.Args) == 3 {
		if err := os.Setenv("ATERM_DATA_DIR", os.Args[2]); err != nil {
			log.Fatal(err)
		}
	}
	if err := app.Run(assets, background); err != nil {
		log.Fatal(err)
	}
}
