//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opendatahub-io/odh-platform-chaos/internal/cli"
	"github.com/spf13/cobra/doc"
)

func main() {
	tmpDir, err := os.MkdirTemp("", "cli-docs-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mktemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	root := cli.NewRootCommand()
	root.DisableAutoGenTag = true
	if err := doc.GenMarkdownTree(root, tmpDir); err != nil {
		fmt.Fprintf(os.Stderr, "gen docs: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("# CLI Reference")
	fmt.Println()
	fmt.Println("Auto-generated from cobra command definitions.")
	fmt.Println()

	entries, _ := os.ReadDir(tmpDir)
	var files []string
	for _, e := range entries {
		files = append(files, e.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(tmpDir, name))
		if err != nil {
			continue
		}
		content := string(data)
		if idx := strings.Index(content, "## SEE ALSO"); idx != -1 {
			content = content[:idx]
		}
		fmt.Println(strings.TrimSpace(content))
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
	}
}
