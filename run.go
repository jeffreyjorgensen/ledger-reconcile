package reconcile

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Input is everything the checks can be given at once.
//
// Every field is optional, and a check whose input is missing does not run.
// What it must never do is run on a guess: see Report.Skipped, which is why
// this type exists at all rather than a dozen separate calls.
type Input struct {
	Operations []Operation      `json:"operations,omitempty"`
	Records    []ExternalRecord `json:"records,omitempty"`
	Quotes     []Quote          `json:"quotes,omitempty"`
	Balances   []Balance        `json:"balances,omitempty"`

	// Opening is the state of every internal balance before the first
	// operation. It is separate from Balances, which is a snapshot of where
	// things stand now: the replay needs where they started, and the two are
	// different facts that happen to have the same shape.
	Opening []Balance `json:"opening,omitempty"`

	Assets   AssetRules    `json:"assets,omitempty"`
	Networks []string      `json:"networks,omitempty"`
	Finality FinalityRules `json:"finality,omitempty"`

	Options Options `json:"options,omitempty"`
}

// Options are the judgements the caller has to make and the library must not.
type Options struct {
	// Zone is the reporting time zone, as an IANA name such as
	// "Europe/Berlin". Without it the day-boundary check does not run.
	Zone string `json:"zone,omitempty"`

	// Counter is the currency operations are valued in for the rate check.
	Counter Currency `json:"counter,omitempty"`

	// DuplicateWindow is how close two lookalike payments must be before they
	// are worth mentioning, as a Go duration such as "5m". Empty or zero
	// leaves only the certain half of the duplicate check running.
	DuplicateWindow string `json:"duplicate_window,omitempty"`

	// Tolerance is how far a figure may drift before it is reported, keyed by
	// currency, with the amount as a decimal string. An absent currency means
	// zero, so an unconfigured tolerance makes the checks louder, never
	// quieter.
	Tolerance map[Currency]string `json:"tolerance,omitempty"`
}

// Skip records a check that did not run and the input that was missing.
//
// A run that quietly performed half the checks would make the same false claim
// a README promising uncovered modes would: that something was verified when
// it was not. Every caller is told what was not looked at.
type Skip struct {
	Check  string `json:"check"`
	Mode   Mode   `json:"mode"`
	Reason string `json:"reason"`
}

// Report is what one run produced and what it could not look at.
type Report struct {
	Findings []Finding `json:"findings"`
	Ran      []string  `json:"ran"`
	Skipped  []Skip    `json:"skipped,omitempty"`
}

// check is one detector together with the reason it might not run.
type check struct {
	name string
	mode Mode
	// skip returns a non-empty reason when the input for this check is absent.
	skip func(Input) string
	run  func(Input, Tolerance, *time.Location, time.Duration) ([]Finding, error)
}

// Run performs every check the input supports and reports what it skipped.
func (in Input) Run() (Report, error) {
	tolerance, err := in.tolerance()
	if err != nil {
		return Report{}, err
	}
	zone, err := in.zone()
	if err != nil {
		return Report{}, err
	}
	window, err := in.window()
	if err != nil {
		return Report{}, err
	}

	report := Report{}
	for _, c := range checks() {
		if reason := c.skip(in); reason != "" {
			report.Skipped = append(report.Skipped, Skip{Check: c.name, Mode: c.mode, Reason: reason})
			continue
		}
		findings, err := c.run(in, tolerance, zone, window)
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", c.name, err)
		}
		report.Ran = append(report.Ran, c.name)
		report.Findings = append(report.Findings, findings...)
	}
	report.Findings = sortFindings(report.Findings)
	return report, nil
}

// The skip predicates. Each names one missing input and nothing else; a check
// that needs two of them composes with allOf, so that the reason a caller sees
// is the first thing actually absent rather than a summary of everything.

func needOperations(in Input) string {
	if len(in.Operations) == 0 {
		return "no operations given"
	}
	return ""
}

func needRecords(in Input) string {
	if len(in.Records) == 0 {
		return "no external records given"
	}
	return ""
}

func needBalances(in Input) string {
	if len(in.Balances) == 0 {
		return "no balances given"
	}
	return ""
}

func needOpening(in Input) string {
	if len(in.Opening) == 0 {
		return "no opening balances given; replaying from an assumed zero would report " +
			"every account funded before this window as overdrawn"
	}
	return ""
}

func needZone(in Input) string {
	if in.Options.Zone == "" {
		return "no reporting time zone given; assuming UTC would silence this check " +
			"exactly when the reporting boundary differs from it"
	}
	return ""
}

func needQuotes(in Input) string {
	if len(in.Quotes) == 0 {
		return "no rate quotes given"
	}
	if in.Options.Counter == "" {
		return "no counter currency given to value operations in"
	}
	return ""
}

// allOf returns the first reason any of the predicates gives.
func allOf(predicates ...func(Input) string) func(Input) string {
	return func(in Input) string {
		for _, predicate := range predicates {
			if reason := predicate(in); reason != "" {
				return reason
			}
		}
		return ""
	}
}

// checks lists every detector in a fixed order.
func checks() []check {
	needBoth := allOf(needOperations, needRecords)

	return []check{
		{"conserve", ModeFeeInOtherAsset, needOperations,
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Conserve(in.Operations)
			}},
		{"fee-conventions", ModeFeeConvention, needOperations,
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return FeeConventions(in.Operations)
			}},
		{"fee-estimates", ModeFeeEstimate, needOperations,
			func(in Input, t Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return FeeEstimates(in.Operations, t)
			}},
		{"replay", ModeMissingHold, allOf(needOperations, needOpening),
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Replay(in.Operations, in.Opening)
			}},
		{"reversals", ModeRefundAsNew, needOperations,
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Reversals(in.Operations)
			}},
		{"daily", ModeDayBoundary, allOf(needOperations, needZone),
			func(in Input, _ Tolerance, zone *time.Location, _ time.Duration) ([]Finding, error) {
				return Daily(in.Operations, zone)
			}},
		{"rates", ModeRateAsOf, allOf(needOperations, needQuotes),
			func(in Input, t Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Rates(in.Operations, in.Quotes, in.Options.Counter, t)
			}},
		{"boundary", ModeInternalMovement, needBoth,
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Boundary(in.Operations, in.Records)
			}},
		{"batches", ModeBatchedTransfer, needBoth,
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Batches(in.Operations, in.Records)
			}},
		{"finality", ModeFinality, needBoth,
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Finality(in.Operations, in.Records, in.Finality)
			}},
		{"duplicates", ModeDoublePayment, needRecords,
			func(in Input, _ Tolerance, _ *time.Location, window time.Duration) ([]Finding, error) {
				return Duplicates(in.Records, window)
			}},
		{"configure", ModeUnwithdrawable, needBalances,
			func(in Input, _ Tolerance, _ *time.Location, _ time.Duration) ([]Finding, error) {
				return Configure(in.Balances, in.Assets, in.Networks)
			}},
	}
}

// tolerance parses the configured drifts, refusing anything unreadable.
func (in Input) tolerance() (Tolerance, error) {
	if len(in.Options.Tolerance) == 0 {
		return nil, nil
	}
	currencies := make([]Currency, 0, len(in.Options.Tolerance))
	for currency := range in.Options.Tolerance {
		currencies = append(currencies, currency)
	}
	sort.Slice(currencies, func(i, j int) bool { return currencies[i] < currencies[j] })

	tolerance := Tolerance{}
	for _, currency := range currencies {
		amount, err := ParseAmount(in.Options.Tolerance[currency], currency)
		if err != nil {
			return nil, fmt.Errorf("options.tolerance: %w", err)
		}
		tolerance[currency] = amount
	}
	return tolerance, nil
}

// zone loads the reporting time zone, if one was given.
func (in Input) zone() (*time.Location, error) {
	if in.Options.Zone == "" {
		return nil, nil
	}
	zone, err := time.LoadLocation(in.Options.Zone)
	if err != nil {
		return nil, fmt.Errorf("options.zone: %q: %w", in.Options.Zone, err)
	}
	return zone, nil
}

// window parses the duplicate window, if one was given.
func (in Input) window() (time.Duration, error) {
	if in.Options.DuplicateWindow == "" {
		return 0, nil
	}
	window, err := time.ParseDuration(in.Options.DuplicateWindow)
	if err != nil {
		return 0, fmt.Errorf("options.duplicate_window: %q: %w", in.Options.DuplicateWindow, err)
	}
	if window < 0 {
		return 0, fmt.Errorf("options.duplicate_window: %q is negative", in.Options.DuplicateWindow)
	}
	return window, nil
}

// String renders the report for a terminal: what was found, then what was not
// looked at. The second half is not an afterthought — a run that checked four
// things out of twelve and said only "4 findings" would be worse than useless.
func (r Report) String() string {
	var b strings.Builder

	if len(r.Findings) == 0 {
		b.WriteString("No findings.\n")
	} else {
		fmt.Fprintf(&b, "%d finding(s):\n\n", len(r.Findings))
		for _, finding := range r.Findings {
			b.WriteString(finding.String())
			b.WriteString("\n\n")
		}
	}

	fmt.Fprintf(&b, "Checks run (%d): %s\n", len(r.Ran), strings.Join(r.Ran, ", "))
	if len(r.Skipped) == 0 {
		return b.String()
	}

	fmt.Fprintf(&b, "\nNot checked (%d):\n", len(r.Skipped))
	for _, skip := range r.Skipped {
		fmt.Fprintf(&b, "  %s [%s] — %s\n", skip.Check, skip.Mode.Number(), skip.Reason)
	}
	return b.String()
}
