package main

import (
	"fmt"
	"os"

	"github.com/fertile-org/banyan/cmd/banyan-engine/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
