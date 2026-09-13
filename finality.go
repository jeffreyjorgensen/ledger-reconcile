package reconcile

import "fmt"

// FinalityRule says when a record on one chain may be treated as irreversible.
//
// Two kinds of chain need two different answers, and conflating them is the
// mode. On a chain that reorganises, depth is the only available proxy and the
// operator picks a number. On a chain that publishes finality, depth says
// nothing at all — a transaction ten blocks deep may still be dropped, and one
// the chain has finalised will not be, however shallow it is.
type FinalityRule struct {
	// Depth is the number of confirmations the operator treats as irreversible
	// on a chain that offers nothing better.
	Depth int

	// ByCheckpoint marks a chain that declares finality itself. Records on such
	// a chain are judged by Final and never by Depth.
	ByCheckpoint bool
}

// FinalityRules maps a chain name to the rule that governs it.
type FinalityRules map[string]FinalityRule

// Finality reports mode 09: a confirmed deposit disappeared.
//
// A credit is a promise to the customer that the money is theirs. Making that
// promise on the strength of a confirmation count, on a chain where the count
// does not mean what the operator thinks it means, is how a deposit that was
// shown as settled stops existing.
//
// A chain with no rule configured is reported rather than passed. There is no
// safe default: a number chosen here would be a number nobody decided, and
// silence would be a claim of irreversibility this library cannot make.
func Finality(operations []Operation, records []ExternalRecord, rules FinalityRules) ([]Finding, error) {
	byRef, err := indexByRef(operations)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, record := range sortedRecords(records) {
		if record.Chain == "" {
			continue
		}
		operation, known := byRef[record.Ref]
		if !known || !creditsInternalBalance(operation) {
			continue
		}
		if finding, ok := finalityFinding(operation, record, rules); ok {
			findings = append(findings, finding)
		}
	}
	return sortFindings(findings), nil
}

// creditsInternalBalance reports whether the operation put value into an
// account inside the ledger — that is, whether it promised anything.
func creditsInternalBalance(operation Operation) bool {
	for _, leg := range operation.Legs {
		if leg.Account.Kind.IsInternal() && leg.Part == BalanceAvailable && leg.Amount.Sign() > 0 {
			return true
		}
	}
	return false
}

// finalityFinding judges one record against its chain's rule.
func finalityFinding(operation Operation, record ExternalRecord, rules FinalityRules) (Finding, bool) {
	rule, configured := rules[record.Chain]

	switch {
	case record.Reorged:
		return reorgFinding(operation, record), true

	case !configured:
		summary := fmt.Sprintf("credited from %s with no finality rule configured for that chain",
			record.Chain)
		arithmetic := fmt.Sprintf(
			"record %s: %d confirmation(s), final = %v. No rule for %q, so nothing "+
				"established that this could not be rolled back",
			record.ID, record.Confirmations, record.Final, record.Chain)
		return newFinding(ModeFinality, operation.ID, record.Amount.Currency(),
			summary, arithmetic, nil), true

	case rule.ByCheckpoint && !record.Final:
		summary := fmt.Sprintf("credited before %s declared the transaction final", record.Chain)
		arithmetic := fmt.Sprintf(
			"record %s: final = false on a chain that publishes finality. Its %d "+
				"confirmation(s) are not an answer to this question",
			record.ID, record.Confirmations)
		return newFinding(ModeFinality, operation.ID, record.Amount.Currency(),
			summary, arithmetic, nil), true

	case !rule.ByCheckpoint && record.Confirmations < rule.Depth:
		summary := fmt.Sprintf("credited at %d confirmation(s) where %s requires %d",
			record.Confirmations, record.Chain, rule.Depth)
		arithmetic := fmt.Sprintf("record %s: %d < %d confirmations required on %s",
			record.ID, record.Confirmations, rule.Depth, record.Chain)
		return newFinding(ModeFinality, operation.ID, record.Amount.Currency(),
			summary, arithmetic, nil), true
	}

	return Finding{}, false
}

// reorgFinding reports a credit whose funding transaction is no longer on the
// chain. This is the mode having already happened rather than being predicted.
func reorgFinding(operation Operation, record ExternalRecord) Finding {
	summary := fmt.Sprintf("credit stands although %s no longer carries the transaction that funded it",
		record.Chain)
	arithmetic := fmt.Sprintf(
		"record %s for %s was reorganised out of %s; the ledger still shows the credit from %s",
		record.ID, record.Amount, record.Chain, operation.ID)

	return newFinding(ModeFinality, operation.ID, record.Amount.Currency(), summary, arithmetic, nil)
}
