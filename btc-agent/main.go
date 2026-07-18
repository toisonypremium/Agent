package main

import (
	"context"
	"log"
	"os"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
