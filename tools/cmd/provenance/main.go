// Command provenance checks that every recipe and probe names where its
// captured output came from, and proves it against upstream where it can.
//
//	go -C tools run ./cmd/provenance [--offline] [--cache DIR] ROOT
//
// It exits 1 when anything is refused, 2 when the check itself could not run.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dispelnet/recipes/tools/internal/provenance"
	"github.com/dispelnet/recipes/tools/internal/upstream"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("provenance", flag.ContinueOnError)
	flags.SetOutput(stderr)

	offline := flags.Bool("offline", false, "skip fetching upstream files; every other rule still applies")
	cache := flags.String("cache", defaultCache(), "directory for fetched upstream files (empty disables caching)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	root := "."
	if flags.NArg() > 1 {
		_, _ = fmt.Fprintln(stderr, "usage: provenance [--offline] [--cache DIR] [ROOT]")

		return 2
	} else if flags.NArg() == 1 {
		root = flags.Arg(0)
	}

	var fetch upstream.Fetcher
	if !*offline {
		fetch = upstream.NewGitHub(*cache)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	report, err := provenance.Check(ctx, os.DirFS(root), fetch)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "provenance: %v\n", err)

		return 2
	}

	annotate := os.Getenv("GITHUB_ACTIONS") == "true"

	for _, problem := range report.Problems {
		_, _ = fmt.Fprintln(stderr, problem)

		if annotate {
			_, _ = fmt.Fprintf(stdout, "::error file=%s::%s\n", escape(problem.Path), escape(problem.Message))
		}
	}

	upstreamLine := fmt.Sprintf("%d proven against upstream", report.Proven)
	if *offline {
		upstreamLine = fmt.Sprintf("%d upstream captures not fetched (--offline)", report.Unchecked)
	}

	_, _ = fmt.Fprintf(stdout, "%d recipes and probes: %s, %d reviewer-attested (modified), %d lab captures (attested by `verified:`)\n",
		report.Files, upstreamLine, len(report.Attested), report.Lab)

	for _, name := range report.Attested {
		_, _ = fmt.Fprintf(stdout, "  attested, not proven: %s\n", name)
	}

	if len(report.Problems) > 0 {
		_, _ = fmt.Fprintf(stderr, "%d problems\n", len(report.Problems))

		return 1
	}

	return 0
}

func defaultCache() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, "dispelnet-recipes", "upstream")
}

// escape encodes a value for a GitHub Actions workflow command.
func escape(value string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(value)
}
