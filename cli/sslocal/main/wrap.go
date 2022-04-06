package main

import (
	"fmt"
	"os"

	"github.com/sagernet/sing/cli/sslocal"
)

func main() {
	err := sslocal.MainCmd().Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
