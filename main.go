package main

import (
	"embed"
	"log"
	"os"
	"ssh-connect/internal/app"
	"ssh-connect/internal/desktop/plugins"
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
	if err := app.Run(assets); err != nil {
		log.Fatal(err)
	}
}
