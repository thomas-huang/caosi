package main

import (
	"os"

	"github.com/thomas-huang/caosi/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args, os.Stdout, os.Stderr))
}
