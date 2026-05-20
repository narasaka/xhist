package main

import (
	"context"
	"fmt"
	"os"

	"github.com/narasaka/xhist/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"error\":%q}\n", err.Error())
		os.Exit(1)
	}
}
