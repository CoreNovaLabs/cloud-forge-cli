package main

import (
	"context"
	"os"

	"github.com/cloud-forge/cli/internal/cli"
)

func main() {
	os.Exit(cli.RunWithIO(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
