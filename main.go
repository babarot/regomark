// regomark checks Markdown documents with rego policies.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/babarot/regomark/internal/cli"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "regomark:", err)
		os.Exit(cli.ExitError)
	}
	os.Exit(cli.Run(context.Background(), os.Args[1:], dir, os.Stdout, os.Stderr))
}
