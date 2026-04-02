// Package main is the entry point for the ytr binary.
package main

import (
	"os"

	"github.com/slavkluev/ytr/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
