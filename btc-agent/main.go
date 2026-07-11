package main

import (
	"context"
	"log"
	"os"
)

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
