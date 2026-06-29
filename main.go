package main

import (
	"os"

	"shd/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
