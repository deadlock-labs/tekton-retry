package main

import (
	"os"

	"github.com/ford-mstech/pipeline-retry/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
