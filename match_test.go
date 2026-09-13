package reconcile

import (
	"strings"
	"testing"
	"time"
)

func payout(id, ref, amount, counterparty string, hour, minute int) ExternalRecord {
	return ExternalRecord{
		ID: id, Ref: ref, Counterparty: counterparty,
		Amount: MustParseAmount("-"+amount, usdt),
		At:     at(2, hour, minute),
	}
}

const fiveMinutes = 5 * time.Minute

func TestDuplicates_DistinctPayments_ReportNothing(t *testing.T) {
	records := []ExternalRecord{
		payout("tx-1", "pay:1", "40", "addr-alice", 9, 0),
		payout("tx-2", "pay:2", "70", "addr-bob", 9, 2),
	}

	findings, err := Duplicates(records, fiveMinutes)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// The unambiguous shape: one reference settled twice.
func TestDuplicates_OneReferenceSettledTwice_ReportsItAsFact(t *testing.T) {
	records := []ExternalRecord{
		payout("tx-1", "pay:1", "40", "addr-alice", 9, 0),
		payout("tx-2", "pay:1", "40", "addr-alice", 9, 1),
	}

	findings, err := Duplicates(records, 0)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Mode != ModeDoublePayment {
		t.Errorf("mode = %s, want %s", findings[0].Mode, ModeDoublePayment)
	}
	if !strings.Contains(findings[0].Summary, "settled 2 times") {
		t.Errorf("summary = %q", findings[0].Summary)
	}
}

// The shape that actually happens: the retry was issued under a fresh key, so
// the two share no reference and only look alike. The finding must say it is a
// suspicion, because two genuine payments of one amount to one recipient are
// ordinary.
func TestDuplicates_RetryUnderANewReference_ReportsASuspicionNotAFact(t *testing.T) {
	records := []ExternalRecord{
		payout("tx-1", "pay:1", "40", "addr-alice", 9, 0),
		payout("tx-2", "pay:2", "40", "addr-alice", 9, 2),
	}

	findings, err := Duplicates(records, fiveMinutes)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, "possibly one request retried, possibly two payments") {
		t.Errorf("the ambiguity must be in the summary: %q", findings[0].Summary)
	}
	if !strings.Contains(findings[0].Arithmetic, "This is a suspicion") {
		t.Errorf("arithmetic = %q", findings[0].Arithmetic)
	}
}

func TestDuplicates_LookalikesFurtherApartThanTheWindow_ReportNothing(t *testing.T) {
	records := []ExternalRecord{
		payout("tx-1", "pay:1", "40", "addr-alice", 9, 0),
		payout("tx-2", "pay:2", "40", "addr-alice", 9, 30),
	}

	findings, err := Duplicates(records, fiveMinutes)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// How close is close enough is the caller's judgement. A zero window turns the
// uncertain check off and leaves only the certain one.
func TestDuplicates_ZeroWindow_LeavesOnlyTheCertainCheck(t *testing.T) {
	records := []ExternalRecord{
		payout("tx-1", "pay:1", "40", "addr-alice", 9, 0),
		payout("tx-2", "pay:2", "40", "addr-alice", 9, 0),
		payout("tx-3", "pay:3", "70", "addr-bob", 9, 0),
		payout("tx-4", "pay:3", "70", "addr-bob", 9, 1),
	}

	findings, err := Duplicates(records, 0)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected only the shared reference, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, `"pay:3"`) {
		t.Errorf("summary = %q", findings[0].Summary)
	}
}

// The same pair must not be reported twice — once as a shared reference and
// again as a lookalike.
func TestDuplicates_SharedReferenceIsNotAlsoReportedAsALookalike(t *testing.T) {
	records := []ExternalRecord{
		payout("tx-1", "pay:1", "40", "addr-alice", 9, 0),
		payout("tx-2", "pay:1", "40", "addr-alice", 9, 1),
	}

	findings, err := Duplicates(records, fiveMinutes)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
}

// batched builds one operation that claims to be settled by a shared chain
// transaction.
func batched(id, amount, batch string) Operation {
	return Operation{
		ID: id, Kind: OpWithdrawal, At: at(2, 9, 0), Ref: "withdrawal:" + id, BatchRef: batch,
		Legs: []Leg{
			leg("l1", userAccount, "-"+amount, usdt, RolePrincipal),
			leg("l2", chainAccount, amount, usdt, RolePrincipal),
		},
	}
}

func TestBatches_GroupMatchesTheTransaction_ReportsNothing(t *testing.T) {
	operations := []Operation{
		batched("op-1", "40", "tx-batch"),
		batched("op-2", "70", "tx-batch"),
		batched("op-3", "10", "tx-batch"),
	}
	record := ExternalRecord{ID: "tx-batch", Ref: "tx-batch",
		Amount: MustParseAmount("-120", usdt), At: at(2, 9, 5)}

	findings, err := Batches(operations, []ExternalRecord{record})
	if err != nil {
		t.Fatalf("Batches: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// Matching one side to the other a record at a time would find three
// discrepancies here. There is one, and it is about the group.
func TestBatches_TransactionCarriedLessThanClaimed_ReportsOneFindingForTheGroup(t *testing.T) {
	operations := []Operation{
		batched("op-1", "40", "tx-batch"),
		batched("op-2", "70", "tx-batch"),
		batched("op-3", "10", "tx-batch"),
	}
	record := ExternalRecord{ID: "tx-batch", Ref: "tx-batch",
		Amount: MustParseAmount("-110", usdt), At: at(2, 9, 5)}

	findings, err := Batches(operations, []ExternalRecord{record})
	if err != nil {
		t.Fatalf("Batches: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding for the group, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeBatchedTransfer {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeBatchedTransfer)
	}
	if !strings.Contains(finding.Summary, "carried 3 operation(s)") {
		t.Errorf("summary must count the group: %q", finding.Summary)
	}
	if !strings.Contains(finding.Arithmetic, "settled -110 - claimed -120 = 10") {
		t.Errorf("arithmetic = %q", finding.Arithmetic)
	}
	if !strings.Contains(finding.Arithmetic, "op-1 op-2 op-3") {
		t.Errorf("arithmetic must name the group: %q", finding.Arithmetic)
	}
}

func TestBatches_NoTransactionForTheGroup_ReportsTheWholeClaim(t *testing.T) {
	operations := []Operation{batched("op-1", "40", "tx-missing")}

	findings, err := Batches(operations, nil)
	if err != nil {
		t.Fatalf("Batches: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Arithmetic, "settled 0 - claimed -40 = 40") {
		t.Errorf("arithmetic = %q", findings[0].Arithmetic)
	}
}

func TestBatches_OperationsWithoutABatch_AreLeftAlone(t *testing.T) {
	findings, err := Batches([]Operation{withdrawalBalanced()}, nil)
	if err != nil {
		t.Fatalf("Batches: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("an unbatched operation is not this detector's business; got:\n%s", render(findings))
	}
}

func TestBatches_MalformedOperation_ReturnsError(t *testing.T) {
	operation := batched("op-1", "40", "tx-batch")
	operation.Legs[0].Account.Kind = AccountUnknown

	if _, err := Batches([]Operation{operation}, nil); err == nil {
		t.Fatal("expected an error for an unclassified account")
	}
}
