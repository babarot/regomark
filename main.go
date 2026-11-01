// regomark checks Markdown documents with rego policies.
package main

import (
	"fmt"
	"os"

	"github.com/babarot/regomark/internal/version"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println("regomark", version.String())
		return
	}
	fmt.Fprintln(os.Stderr, "usage: regomark version")
	os.Exit(2)
}
