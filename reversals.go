package reconcile

import (
	"fmt"
	"sort"
)

// Reversals reports mode 06: turnover doubled out of nowhere.
//
// A refund that is recorded as a fresh payment in the opposite direction is
// arithmetically fine — the balances all come out right — and doubles every
// figure built by adding movements up rather than netting them. Turnover,
// volume, fee revenue projected from volume, the number quoted to an investor:
// all of them inflate, and no single entry is wrong.
//
// This needs the operation to say what it reverses. The alternative is a
// heuristic — same counterparty, opposite sign, close amount, near in time —
// and that heuristic fires on a customer who pays and is refunded in the
// ordinary course of business. A false finding costs more here than a missed
// one, because the whole claim of this library is that its findings are
// defensible. So a refund with no link is not guessed at.
func Reversals(operations []Operation) ([]Finding, error) {
	byID := map[string]Operation{}
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		if previous, clash := byID[operation.ID]; clash {
			return nil, fmt.Errorf("reversals: two operations share the id %q (%s)",
				operation.ID, previous.Kind)
		}
		byID[operation.ID] = operation
	}

	gross, net := NewSum(), NewSum()
	var pairs []string
	var findings []Finding

	for _, operation := range operations {
		crossing := crossingSum(operation)
		for _, currency := range crossing.Currencies() {
			gross.Add(crossing.In(currency).Abs())
		}

		if operation.Reverses == "" {
			for _, currency := range crossing.Currencies() {
				net.Add(crossing.In(currency).Abs())
			}
			continue
		}

		original, known := byID[operation.Reverses]
		if !known {
			findings = append(findings, danglingReversalFinding(operation))
			continue
		}
		if finding, mismatched := mirrorFinding(operation, original); mismatched {
			findings = append(findings, finding)
		}
		pairs = append(pairs, original.ID+" ← "+operation.ID)

		// A reversal and its original net to nothing, so neither is added to
		// the netted figure. The pair moved value twice and produced none.
		for _, currency := range crossing.Currencies() {
			net.Add(crossing.In(currency).Abs().Neg())
		}
	}

	sort.Strings(pairs)
	findings = append(findings, inflationFindings(gross, net, pairs)...)
	return sortFindings(findings), nil
}

// crossingSum is what one operation moved across the ledger's boundary, per
// currency, signed from the ledger's point of view.
func crossingSum(operation Operation) *Sum {
	crossing := NewSum()
	for _, leg := range operation.Legs {
		if isExternal(leg.Account) {
			crossing.Add(leg.Amount.Neg())
		}
	}
	return crossing
}

// danglingReversalFinding reports a reversal whose original is not in the set.
func danglingReversalFinding(operation Operation) Finding {
	crossing := crossingSum(operation)
	currency := Currency("")
	if currencies := crossing.Currencies(); len(currencies) > 0 {
		currency = currencies[0]
	}

	summary := fmt.Sprintf(
		"reverses %s, which is not in this set: the pair cannot be netted and "+
			"turnover will carry both",
		operation.Reverses)
	arithmetic := fmt.Sprintf("%s declares itself a reversal of %s; no such operation was given, "+
		"so nothing here can say whether the two cancel", operation.ID, operation.Reverses)

	return newFinding(ModeRefundAsNew, operation.ID, currency, summary, arithmetic, nil)
}

// mirrorFinding reports a reversal that does not mirror what it claims to undo.
func mirrorFinding(reversal, original Operation) (Finding, bool) {
	reversed, originally := crossingSum(reversal), crossingSum(original)
	for _, currency := range union(reversed, originally) {
		expected := originally.In(currency).Neg()
		if reversed.In(currency).Equal(expected) {
			continue
		}
		summary := fmt.Sprintf("reverses %s but moves %s where undoing it would move %s",
			original.ID, reversed.In(currency), expected)
		arithmetic := fmt.Sprintf("%s moved %s; %s moved %s; a full reversal would have moved %s",
			original.ID, originally.In(currency).Value(),
			reversal.ID, reversed.In(currency).Value(), expected.Value())
		return newFinding(ModeRefundAsNew, reversal.ID, currency, summary, arithmetic, nil), true
	}
	return Finding{}, false
}

// inflationFindings state, per currency, how far a turnover figure that counts
// reversals as fresh operations sits above the netted one.
func inflationFindings(gross, net *Sum, pairs []string) []Finding {
	if len(pairs) == 0 {
		return nil
	}

	var findings []Finding
	for _, currency := range gross.Currencies() {
		inflation, err := gross.In(currency).Sub(net.In(currency))
		if err != nil || inflation.IsZero() {
			continue
		}
		summary := fmt.Sprintf(
			"turnover counted as movements is %s against %s netted: %d reversal(s) are being counted twice",
			gross.In(currency), net.In(currency), len(pairs))
		arithmetic := fmt.Sprintf("gross %s - net %s = %s inflation, from %v",
			gross.In(currency).Value(), net.In(currency).Value(), inflation.Value(), pairs)

		findings = append(findings, newFinding(ModeRefundAsNew, joinOperations(pairs), currency,
			summary, arithmetic, nil))
	}
	return findings
}
