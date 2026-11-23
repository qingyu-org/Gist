package main

import (
	"github.com/qingyu-org/Gist/internal/cli"
	"os"
)

func main() {
	if err := cli.App(); err != nil {
		os.Exit(1)
	}
}
