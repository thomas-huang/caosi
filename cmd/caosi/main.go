package main

import (
	"os"

	"caosi/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args, os.Stdout, os.Stderr))
}
