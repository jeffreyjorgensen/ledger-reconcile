package reconcile

import (
	"fmt"
	"sort"
	"time"
)

// Duplicates reports mode 02: the recipient was paid twice.
//
// Two shapes, and they are not equally certain, so they are not reported as
// though they were.
//
// The first is a request executed twice under one key: two records carrying
// the same reference. Nothing about that is ambiguous.
//
// The second is the one that actually happens. A request times out, the caller
// retries, and the retry is issued under a fresh key — so the two payments
// share no reference at all and only look alike: same counterparty, same
// amount, close together. That is a suspicion, not a fact. Two genuine
// payments of the same amount to the same recipient minutes apart are ordinary,
// and a library that called them a duplicate would be wrong in a way its reader
// could not check. So the finding says what it is and what it is not.
//
// A zero window turns the second check off. It is the caller's judgement, not
// the library's, how close is close enough.
func Duplicates(records []ExternalRecord, window time.Duration) ([]Finding, error) {
	ordered := sortedRecords(records)

	findings := sharedReferenceFindings(ordered)
	if window > 0 {
		findings = append(findings, lookalikeFindings(ordered, window)...)
	}
	return sortFindings(findings), nil
}

// sharedReferenceFindings reports references that settled more than once.
func sharedReferenceFindings(records []ExternalRecord) []Finding {
	byReference := map[string][]ExternalRecord{}
	var references []string
	for _, record := range records {
		if record.Ref == "" {
			continue
		}
		if _, seen := byReference[record.Ref]; !seen {
			references = append(references, record.Ref)
		}
		byReference[record.Ref] = append(byReference[record.Ref], record)
	}
	sort.Strings(references)

	var findings []Finding
	for _, reference := range references {
		group := byReference[reference]
		if len(group) < 2 {
			continue
		}
		findings = append(findings, sharedReferenceFinding(reference, group))
	}
	return findings
}

func sharedReferenceFinding(reference string, group []ExternalRecord) Finding {
	total := NewSum()
	for _, record := range group {
		total.Add(record.Amount)
	}

	summary := fmt.Sprintf("reference %q settled %d times: the same request was executed more than once",
		reference, len(group))
	arithmetic := fmt.Sprintf("records %v carry %s in total where the reference should have moved %s once",
		recordIDs(group), total.In(group[0].Amount.Currency()).Value(), group[0].Amount)

	return newFinding(ModeDoublePayment, reference, group[0].Amount.Currency(),
		summary, arithmetic, nil)
}

// lookalike groups records that cannot be told apart by anything except the
// reference they were issued under.
type lookalike struct {
	counterparty string
	amount       string
	currency     Currency
}

// lookalikeFindings reports pairs that look like one payment issued twice.
func lookalikeFindings(records []ExternalRecord, window time.Duration) []Finding {
	byShape := map[lookalike][]ExternalRecord{}
	var shapes []lookalike
	for _, record := range records {
		if record.Counterparty == "" || record.At.IsZero() {
			continue
		}
		key := lookalike{
			counterparty: record.Counterparty,
			amount:       record.Amount.Value().String(),
			currency:     record.Amount.Currency(),
		}
		if _, seen := byShape[key]; !seen {
			shapes = append(shapes, key)
		}
		byShape[key] = append(byShape[key], record)
	}
	sort.Slice(shapes, func(i, j int) bool {
		if shapes[i].counterparty != shapes[j].counterparty {
			return shapes[i].counterparty < shapes[j].counterparty
		}
		return shapes[i].amount < shapes[j].amount
	})

	var findings []Finding
	for _, shape := range shapes {
		findings = append(findings, closePairFindings(shape, byShape[shape], window)...)
	}
	return findings
}

// closePairFindings reports consecutive records of one shape that fall inside
// the window.
func closePairFindings(shape lookalike, group []ExternalRecord, window time.Duration) []Finding {
	if len(group) < 2 {
		return nil
	}
	inTime := make([]ExternalRecord, len(group))
	copy(inTime, group)
	sort.SliceStable(inTime, func(i, j int) bool { return inTime[i].At.Before(inTime[j].At) })

	var findings []Finding
	for i := 1; i < len(inTime); i++ {
		first, second := inTime[i-1], inTime[i]
		if first.Ref != "" && first.Ref == second.Ref {
			continue // already reported as one reference settling twice.
		}
		apart := second.At.Sub(first.At)
		if apart > window {
			continue
		}
		findings = append(findings, lookalikeFinding(shape, first, second, apart, window))
	}
	return findings
}

func lookalikeFinding(shape lookalike, first, second ExternalRecord,
	apart, window time.Duration) Finding {
	summary := fmt.Sprintf(
		"%s and %s pay %s to %s %s apart under different references: possibly one request retried, possibly two payments",
		first.ID, second.ID, second.Amount, shape.counterparty, apart)
	arithmetic := fmt.Sprintf(
		"%s at %s (ref %q) and %s at %s (ref %q): same counterparty and amount, %s apart, window %s. "+
			"This is a suspicion — two genuine payments of one amount to one recipient are ordinary",
		first.ID, first.At.UTC().Format(time.RFC3339), first.Ref,
		second.ID, second.At.UTC().Format(time.RFC3339), second.Ref, apart, window)

	return newFinding(ModeDoublePayment, second.ID, shape.currency, summary, arithmetic, nil)
}

// Batches reports mode 10: one transaction is not one operation.
//
// A batched payout is one transaction on the chain and twenty operations in
// the ledger; a sweep is one transaction and none. Matching either side to the
// other one record at a time finds twenty discrepancies where there are none,
// or one where there are twenty. The group is the unit, and the finding is
// about the group.
func Batches(operations []Operation, records []ExternalRecord) ([]Finding, error) {
	grouped, batches, err := groupByBatch(operations)
	if err != nil {
		return nil, err
	}

	settled := map[string]*Sum{}
	for _, record := range sortedRecords(records) {
		if _, batched := grouped[record.Ref]; !batched {
			continue
		}
		if settled[record.Ref] == nil {
			settled[record.Ref] = NewSum()
		}
		settled[record.Ref].Add(record.Amount)
	}

	var findings []Finding
	for _, batch := range batches {
		findings = append(findings, batchFindings(batch, grouped[batch], settled[batch])...)
	}
	return sortFindings(findings), nil
}

// groupByBatch collects the operations that claim each external transaction.
func groupByBatch(operations []Operation) (map[string][]Operation, []string, error) {
	grouped := map[string][]Operation{}
	var batches []string
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, nil, err
		}
		if operation.BatchRef == "" {
			continue
		}
		if _, seen := grouped[operation.BatchRef]; !seen {
			batches = append(batches, operation.BatchRef)
		}
		grouped[operation.BatchRef] = append(grouped[operation.BatchRef], operation)
	}
	sort.Strings(batches)
	return grouped, batches, nil
}

// batchFindings compares what one group of operations claims crossed the
// boundary with what the transaction carrying them actually moved.
func batchFindings(batch string, operations []Operation, settled *Sum) []Finding {
	claimed := NewSum()
	for _, operation := range operations {
		for _, leg := range operation.Legs {
			if isExternal(leg.Account) {
				claimed.Add(leg.Amount.Neg())
			}
		}
	}
	if settled == nil {
		settled = NewSum()
	}

	var findings []Finding
	for _, currency := range union(claimed, settled) {
		residual, err := settled.In(currency).Sub(claimed.In(currency))
		if err != nil || residual.IsZero() {
			continue
		}
		findings = append(findings, batchFinding(batch, operations, currency, claimed, settled, residual))
	}
	return findings
}

func batchFinding(batch string, operations []Operation, currency Currency,
	claimed, settled *Sum, residual Amount) Finding {
	summary := fmt.Sprintf(
		"transaction %q carried %d operation(s) claiming %s while the transaction moved %s: %s unaccounted",
		batch, len(operations), claimed.In(currency), settled.In(currency), residual.Abs())
	arithmetic := fmt.Sprintf("settled %s - claimed %s = %s across %v",
		settled.In(currency).Value(), claimed.In(currency).Value(),
		residual.Value(), operationIDs(operations))

	return newFinding(ModeBatchedTransfer, joinOperations(operationIDs(operations)), currency,
		summary, arithmetic, nil)
}
