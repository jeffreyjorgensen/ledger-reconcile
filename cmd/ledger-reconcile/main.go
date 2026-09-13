// Command ledger-reconcile runs the checks in
// github.com/jeffreyjorgensen/ledger-reconcile over a JSON description of a
// ledger and the outside world, and prints what it found and what it could not
// look at.
//
// Usage:
//
//	ledger-reconcile [-in file] [-json]
//
// With no -in, or with "-", the input is read from standard input.
//
// Exit status:
//
//	0  the checks that ran found nothing
//	1  at least one finding
//	2  the input could not be read, or a check refused to run on it
//
// One and two are different outcomes and are worth separate codes: a check
// that could not run is not a check that passed.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	// The day-boundary check needs a real time zone. Embedding the database
	// means the tool behaves the same on a machine that has no system tzdata —
	// a container, most often — instead of failing there and nowhere else.
	_ "time/tzdata"

	reconcile "github.com/jeffreyjorgensen/ledger-reconcile"
)

const (
	exitClean    = 0
	exitFindings = 1
	exitError    = 2
)

func main() {
	in := flag.String("in", "-", `input file, or "-" for standard input`)
	asJSON := flag.Bool("json", false, "print the report as JSON")
	flag.Parse()

	os.Exit(run(*in, *asJSON, os.Stdout, os.Stderr))
}

// run is main's body with its inputs and outputs passed in, so that the whole
// command can be tested without a subprocess.
func run(path string, asJSON bool, stdout, stderr io.Writer) int {
	input, err := read(path)
	if err != nil {
		complain(stderr, err)
		return exitError
	}

	report, err := input.Run()
	if err != nil {
		complain(stderr, err)
		return exitError
	}

	if err := write(report, asJSON, stdout); err != nil {
		complain(stderr, err)
		return exitError
	}

	if len(report.Findings) > 0 {
		return exitFindings
	}
	return exitClean
}

// complain reports a failure on the error stream.
//
// If that write itself fails there is nowhere left to say so, and the exit
// code still carries the outcome, so the error is deliberately dropped.
func complain(stderr io.Writer, err error) {
	_, _ = fmt.Fprintf(stderr, "ledger-reconcile: %v\n", err)
}

// read decodes the input, refusing anything it does not understand rather than
// ignoring it. A misspelled field would otherwise turn into a check that
// silently had nothing to work on.
func read(path string) (reconcile.Input, error) {
	source := io.Reader(os.Stdin)
	if path != "-" && path != "" {
		file, err := os.Open(path)
		if err != nil {
			return reconcile.Input{}, err
		}
		// Closing a file that was only read has nothing useful to report.
		defer func() { _ = file.Close() }()
		source = file
	}

	decoder := json.NewDecoder(source)
	decoder.DisallowUnknownFields()

	var input reconcile.Input
	if err := decoder.Decode(&input); err != nil {
		return reconcile.Input{}, fmt.Errorf("reading input: %w", err)
	}
	return input, nil
}

// write prints the report in whichever form was asked for.
func write(report reconcile.Report, asJSON bool, stdout io.Writer) error {
	if !asJSON {
		_, err := io.WriteString(stdout, report.String())
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
