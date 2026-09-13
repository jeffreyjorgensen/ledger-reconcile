package reconcile

import "fmt"

// Boundary compares what the ledger says crossed its boundary with what the
// outside world recorded, one currency at a time.
//
// Mode 11 is the case this exists for. A sweep between two addresses the
// operator owns is a real transaction on the chain and no movement at all from
// the ledger's point of view: nothing entered, nothing left. Compare chain
// volume against deposits and withdrawals and the two will differ by exactly
// the sweeps, while every single entry on both sides is correct. The usual
// response is to hunt for a wrong figure that does not exist.
//
// When the gap is accounted for by those movements, the finding says so and
// names them. When it is not, the gap is reported as it stands rather than
// explained away.
func Boundary(operations []Operation, records []ExternalRecord) ([]Finding, error) {
	byRef, err := indexByRef(operations)
	if err != nil {
		return nil, err
	}

	crossing, external, internal := NewSum(), NewSum(), NewSum()
	internalOps := map[Currency][]string{}

	for _, operation := range operations {
		for _, leg := range operation.Legs {
			if isExternal(leg.Account) {
				crossing.Add(leg.Amount.Neg())
			}
		}
	}

	// One internal movement explains its own amount once. A second record
	// claiming the same movement is not more internal movement — it is the
	// same one recorded twice, and absorbing it here would launder a duplicate
	// into an explanation. It stays in the external total, where it shows up
	// as a gap nothing accounts for.
	counted := map[string]bool{}
	for _, record := range records {
		external.Add(record.Amount)

		operation, known := byRef[record.Ref]
		switch {
		case !known, crossesBoundary(operation), counted[operation.ID]:
			continue
		default:
			counted[operation.ID] = true
			internal.Add(record.Amount)
			internalOps[record.Amount.Currency()] =
				append(internalOps[record.Amount.Currency()], operation.ID)
		}
	}

	return sortFindings(boundaryFindings(crossing, external, internal, internalOps)), nil
}

// indexByRef maps operations by their idempotency key, rejecting duplicates.
//
// Two operations sharing a key is not a boundary question, but it makes every
// answer here meaningless, so it is refused rather than resolved arbitrarily.
func indexByRef(operations []Operation) (map[string]Operation, error) {
	byRef := make(map[string]Operation, len(operations))
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		if operation.Ref == "" {
			continue
		}
		if previous, clash := byRef[operation.Ref]; clash {
			return nil, fmt.Errorf("boundary: operations %s and %s share the reference %q",
				previous.ID, operation.ID, operation.Ref)
		}
		byRef[operation.Ref] = operation
	}
	return byRef, nil
}

// crossesBoundary reports whether any leg of the operation touches an account
// outside the ledger.
func crossesBoundary(operation Operation) bool {
	for _, leg := range operation.Legs {
		if isExternal(leg.Account) {
			return true
		}
	}
	return false
}

// boundaryFindings reports each currency in which the two sides disagree.
func boundaryFindings(crossing, external, internal *Sum, internalOps map[Currency][]string) []Finding {
	var findings []Finding
	for _, currency := range union(crossing, external) {
		gap, err := external.In(currency).Sub(crossing.In(currency))
		if err != nil || gap.IsZero() {
			continue
		}
		findings = append(findings,
			boundaryFinding(currency, crossing, external, internal, gap, internalOps[currency]))
	}
	return findings
}

// boundaryFinding decides whether one currency's gap is mode 11 or an
// unexplained difference.
func boundaryFinding(currency Currency, crossing, external, internal *Sum,
	gap Amount, operations []string) Finding {
	unexplained := internal.In(currency)
	if gap.Equal(unexplained) && !unexplained.IsZero() {
		summary := fmt.Sprintf(
			"the two sides differ by %s, which is exactly the %d movement(s) that never crossed the boundary",
			gap, len(operations))
		arithmetic := fmt.Sprintf(
			"external %s - crossing %s = %s; movements recorded outside but internal to the ledger = %s (%v)",
			external.In(currency).Value(), crossing.In(currency).Value(),
			gap.Value(), unexplained.Value(), operations)
		return newFinding(ModeInternalMovement, joinOperations(operations), currency,
			summary, arithmetic, nil)
	}

	summary := fmt.Sprintf("the two sides differ by %s and nothing in the input accounts for it", gap)
	arithmetic := fmt.Sprintf("external %s - crossing %s = %s; internal movements explain only %s",
		external.In(currency).Value(), crossing.In(currency).Value(),
		gap.Value(), unexplained.Value())
	return newFinding(ModeUnbalanced, "", currency, summary, arithmetic, nil)
}

// union returns every currency either sum has seen, in a stable order.
func union(a, b *Sum) []Currency {
	combined := NewSum()
	for _, currency := range a.Currencies() {
		combined.Add(zeroAmount(currency))
	}
	for _, currency := range b.Currencies() {
		combined.Add(zeroAmount(currency))
	}
	return combined.Currencies()
}
