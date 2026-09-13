package reconcile

import (
	"strings"
	"testing"
)

// outbound sends value to the chain.
func outbound(id, amount string) Operation {
	return Operation{
		ID: id, Kind: OpWithdrawal, At: at(2, 9, 0), Ref: "withdrawal:" + id,
		Legs: []Leg{
			leg("l1", userAccount, "-"+amount, usdt, RolePrincipal),
			leg("l2", chainAccount, amount, usdt, RolePrincipal),
		},
	}
}

// refundOf returns value, and says what it is returning.
func refundOf(id, amount, original string) Operation {
	operation := Operation{
		ID: id, Kind: OpDeposit, At: at(2, 10, 0), Ref: "refund:" + id, Reverses: original,
		Legs: []Leg{
			leg("l1", chainAccount, "-"+amount, usdt, RolePrincipal),
			leg("l2", userAccount, amount, usdt, RolePrincipal),
		},
	}
	return operation
}

func TestReversals_NothingReversed_ReportsNothing(t *testing.T) {
	findings, err := Reversals([]Operation{outbound("op-1", "40"), outbound("op-2", "70")})
	if err != nil {
		t.Fatalf("Reversals: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// Mode 06. Every balance comes out right. The pair moved 80 and produced
// nothing, and a turnover figure built by adding movements up says 80 where
// the netted figure says 0.
func TestReversals_RefundLinkedToItsOriginal_ReportsTheInflation(t *testing.T) {
	operations := []Operation{
		outbound("op-1", "40"),
		refundOf("op-2", "40", "op-1"),
	}

	findings, err := Reversals(operations)
	if err != nil {
		t.Fatalf("Reversals: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeRefundAsNew {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeRefundAsNew)
	}
	if !strings.Contains(finding.Summary, "counted twice") {
		t.Errorf("summary = %q", finding.Summary)
	}
	if !strings.Contains(finding.Arithmetic, "gross 80 - net 0 = 80 inflation") {
		t.Errorf("arithmetic = %q", finding.Arithmetic)
	}
	if !strings.Contains(finding.Arithmetic, "op-1 ← op-2") {
		t.Errorf("arithmetic must name the pair: %q", finding.Arithmetic)
	}
}

// A refund with no link is not guessed at. The heuristic that would catch it
// also fires on a customer who pays and is refunded in the ordinary course of
// business, and a false finding costs more here than a missed one.
func TestReversals_RefundWithNoLink_IsNotGuessedAt(t *testing.T) {
	unlinked := refundOf("op-2", "40", "")
	unlinked.Reverses = ""

	findings, err := Reversals([]Operation{outbound("op-1", "40"), unlinked})
	if err != nil {
		t.Fatalf("Reversals: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a refund that does not declare itself must not be inferred; got:\n%s", render(findings))
	}
}

func TestReversals_OriginalNotInTheSet_IsReportedAsUnnettable(t *testing.T) {
	findings, err := Reversals([]Operation{refundOf("op-2", "40", "op-elsewhere")})
	if err != nil {
		t.Fatalf("Reversals: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, "not in this set") {
		t.Errorf("summary = %q", findings[0].Summary)
	}
}

// A partial refund recorded as a full reversal is a different defect from a
// refund recorded as a new operation, and it must not be waved through.
func TestReversals_RefundDoesNotMirrorItsOriginal_IsReported(t *testing.T) {
	operations := []Operation{
		outbound("op-1", "40"),
		refundOf("op-2", "25", "op-1"),
	}

	findings, err := Reversals(operations)
	if err != nil {
		t.Fatalf("Reversals: %v", err)
	}

	var mirrored bool
	for _, finding := range findings {
		if strings.Contains(finding.Arithmetic, "a full reversal would have moved") {
			mirrored = true
			if !strings.Contains(finding.Arithmetic, "op-1 moved -40") {
				t.Errorf("arithmetic = %q", finding.Arithmetic)
			}
		}
	}
	if !mirrored {
		t.Fatalf("a reversal that does not mirror its original must be reported; got:\n%s", render(findings))
	}
}

func TestReversals_DuplicateOperationID_ReturnsError(t *testing.T) {
	_, err := Reversals([]Operation{outbound("op-1", "40"), outbound("op-1", "70")})
	if err == nil {
		t.Fatal("two operations sharing an id make every answer here meaningless")
	}
	if !strings.Contains(err.Error(), "share the id") {
		t.Errorf("error = %v", err)
	}
}

func TestReversals_MalformedOperation_ReturnsError(t *testing.T) {
	operation := outbound("op-1", "40")
	operation.Legs[0].Account.Kind = AccountUnknown

	if _, err := Reversals([]Operation{operation}); err == nil {
		t.Fatal("expected an error for an unclassified account")
	}
}
