// Package main is the entry point for the repog CLI.
package main

import (
	"os"

	"github.com/hackastak/repog/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
