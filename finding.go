package reconcile

import (
	"sort"
	"strings"
)

// Evidence is one record that took part in a finding.
//
// Findings carry the records rather than referring to them, because the reader
// of a finding is usually not holding the input file open beside it.
type Evidence struct {
	LegID   string  `json:"leg_id,omitempty"`
	Account Account `json:"account"`
	Role    LegRole `json:"role"`
	Amount  Amount  `json:"amount"`
}

// Finding is one detected discrepancy.
//
// It states the mode it matched, where it was found, the arithmetic that
// failed and the records that produced it. The site this library backs claims
// that a discrepancy should point at where to look rather than merely exist,
// and Arithmetic plus Evidence are how that claim is kept.
type Finding struct {
	Mode      Mode     `json:"mode"`
	ModeTitle string   `json:"mode_title"`
	Article   string   `json:"article"`
	Operation string   `json:"operation"`
	Currency  Currency `json:"currency,omitempty"`

	// Summary is one line naming what is wrong.
	Summary string `json:"summary"`

	// Arithmetic is the calculation written out, so the reader can check it
	// without rerunning the library.
	Arithmetic string `json:"arithmetic"`

	// Evidence is the records the arithmetic was performed on.
	Evidence []Evidence `json:"evidence,omitempty"`
}

// newFinding fills in the fields every finding carries the same way.
func newFinding(mode Mode, operation string, currency Currency,
	summary, arithmetic string, evidence []Evidence) Finding {
	return Finding{
		Mode:       mode,
		ModeTitle:  mode.Title(),
		Article:    mode.ArticleURL(),
		Operation:  operation,
		Currency:   currency,
		Summary:    summary,
		Arithmetic: arithmetic,
		Evidence:   evidence,
	}
}

// findingIndent is the gutter every line after the summary sits in, wide
// enough for the longest label so the values line up under each other.
const findingIndent = "     "

// String renders a finding for a terminal, mode number first.
func (f Finding) String() string {
	var b strings.Builder
	b.WriteString("[" + f.Mode.Number() + "] " + f.Summary)

	b.WriteString(field("operation", f.Operation))
	if f.Currency != "" {
		b.WriteString(field("currency", string(f.Currency)))
	}
	b.WriteString(field("arithmetic", f.Arithmetic))

	for _, e := range f.Evidence {
		b.WriteString(field("leg "+e.LegID, e.Account.ID+
			" ("+e.Account.Kind.String()+", "+e.Role.String()+") "+e.Amount.String()))
	}
	b.WriteString(field("see", f.Article))
	return b.String()
}

// field renders one labelled line of a finding, padded so that the values of a
// finding line up under one another.
func field(label, value string) string {
	const width = 11 // the width of "arithmetic:"
	label += ":"
	for len(label) < width {
		label += " "
	}
	return "\n" + findingIndent + label + " " + value
}

// evidenceFrom turns legs into evidence in the order they were given.
func evidenceFrom(legs []Leg) []Evidence {
	evidence := make([]Evidence, 0, len(legs))
	for _, leg := range legs {
		evidence = append(evidence, Evidence{
			LegID:   leg.ID,
			Account: leg.Account,
			Role:    leg.Role,
			Amount:  leg.Amount,
		})
	}
	return evidence
}

// sortFindings puts findings in an order that depends only on their content.
//
// Every detector runs this before returning. Identical input has to produce an
// identical list, and anything ordered by map iteration would not — which
// would quietly make the library's claim of determinism untrue, and would show
// up first as a test that fails one run in ten.
func sortFindings(findings []Finding) []Finding {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Mode != b.Mode {
			return a.Mode < b.Mode
		}
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		if a.Currency != b.Currency {
			return a.Currency < b.Currency
		}
		return a.Summary < b.Summary
	})
	return findings
}
