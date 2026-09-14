package main

import (
	"flag"

	"github.com/fast-template/springhere-gin-server/pkg/app"
)

func main() {
	path := flag.String("config", "application.yaml", "configuration file path")
	flag.Parse()
	application, cleanups := app.New(*path)
	defer cleanups()
	// start and wait for stop signal
	if err := application.Run(); err != nil {
		panic(err)
	}
}
