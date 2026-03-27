package main

import (
	"os"

	"github.com/josepavese/needlex/internal/transport"
)

func main() {
	runner := transport.NewRunner()
	os.Exit(runner.Run(os.Args[1:], os.Stdout, os.Stderr))
}
