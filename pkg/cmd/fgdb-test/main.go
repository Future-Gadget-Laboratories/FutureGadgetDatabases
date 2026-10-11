// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Command fgdb-test runs one FGDb suite tier.
// The same binary is the history recorder when invoked as `fgdb-test record`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/harness"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/record"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	os.Exit(dispatch(os.Args[1], os.Args[2:]))
}

func dispatch(cmd string, args []string) int {
	switch cmd {
	case "run":
		return runTier(args)
	case "record":
		return record.Run(args)
	case "help", "-h", "--help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `fgdb-test runs the FGDb suite on this machine.

  fgdb-test run --tier pr|nightly|weekly|interim [--root DIR] [--output DIR]
  fgdb-test record --out FILE --urls URLS --duration 20m --clients 3

The shell entry point is fgdb/test/run.sh. It builds this command and calls run.
Partition and disk faults are not run. The report says so in its first lines.
`)
}

func runTier(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tier := fs.String("tier", "", "pr, nightly, weekly, or interim")
	root := fs.String("root", "", "repository root (default: walk up from the working directory)")
	output := fs.String("output", "", "directory for result.json and summary.md")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := execute(context.Background(), *tier, *root, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func execute(ctx context.Context, tier, root, output string) error {
	if tier == "" {
		return fmt.Errorf("run needs --tier")
	}
	found, err := harness.FindRoot(root)
	if err != nil {
		return err
	}
	if output == "" {
		output, err = os.MkdirTemp("", "fgdb-suite-")
		if err != nil {
			return err
		}
	}
	h := &harness.Harness{Root: found, TierName: tier, Output: output}
	fmt.Printf("report directory: %s\n", output)
	return h.Run(ctx)
}
