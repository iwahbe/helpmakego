package main

import (
	"fmt"
	"os"

	"github.com/iwahbe/helpmakego/internal/cmd"
)

func main() {
	if err := cmd.Root().Execute(); err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}
}
