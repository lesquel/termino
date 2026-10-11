// Package main is the entry point for the termino binary.
// It delegates immediately to the CLI root command which wires all
// sub-commands, dependency injection, and startup logic.
package main

import (
	"fmt"
	"os"

	"github.com/lesquel/termino/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
