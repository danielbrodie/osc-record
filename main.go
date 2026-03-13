package main

import (
	"fmt"
	"os"

	"github.com/danielbrodie/osc-record/cmd"
)

var version = "0.1.0"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
