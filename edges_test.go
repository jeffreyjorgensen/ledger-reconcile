package reconcile

import (
	"strings"
	"testing"
	"time"
)

// The edges. Each of these was a hypothesis about a way the library could be
// wrong, written during a read of the whole thing; three of them were right,
// and those three are now the reason this file exists.

// A trade has two principal currencies. PrincipalCurrency returns the
// first in sorted order, so the other one looks like "a different asset" — and
// a residual there with a fee leg would be reported as mode 14, which says the
// fee was paid in an asset outside the operation. It was not.
func TestConserve_TradeShortInItsOtherPrincipalCurrency_IsNotAFeeInAnotherAsset(t *testing.T) {
	operation := Operation{
		ID: "op-trade", Kind: OpTrade, At: fixtureTime,
		Legs: []Leg{
			leg("l1", userAccount, "-2", btc, RolePrincipal),
			leg("l2", treasuryAccount, "2", btc, RolePrincipal),
			leg("l3", userAccount, "100", usdt, RolePrincipal),
			leg("l4", treasuryAccount, "-90", usdt, RolePrincipal), // 10 short
			leg("l5", feeAccount, "1", usdt, RoleFee),              // fee in USDT
		},
	}

	findings, err := Conserve([]Operation{operation})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Mode == ModeFeeInOtherAsset {
		t.Errorf("USDT is a principal currency of this trade, not a different asset; "+
			"reporting mode 14 here says something untrue:\n%s", findings[0])
	}
}

// An operation with no principal legs at all — an adjustment posting
// only fees. PrincipalCurrency is empty, so every currency differs from it and
// any residual with a fee leg becomes mode 14.
func TestConserve_OperationWithNoPrincipalLegs_IsNotAFeeInAnotherAsset(t *testing.T) {
	operation := Operation{
		ID: "op-adjust", Kind: OpAdjustment, At: fixtureTime,
		Legs: []Leg{
			leg("l1", feeAccount, "1", usdt, RoleFee), // nothing pays it
		},
	}

	findings, err := Conserve([]Operation{operation})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Mode == ModeFeeInOtherAsset {
		t.Errorf("there is no other asset here — the operation has one currency:\n%s", findings[0])
	}
}

// Duplicates sorts records by id, then pairs "consecutive" lookalikes
// by time. If two records of one shape are further apart than the window but a
// third sits between them, is each pair judged, or only the sorted-adjacent
// ones? Three payments five minutes apart should give two suspicions, not one.
func TestDuplicates_ThreeLookalikesInARow_ReportEveryClosePair(t *testing.T) {
	records := []ExternalRecord{
		payout("tx-1", "pay:1", "40", "addr-alice", 9, 0),
		payout("tx-2", "pay:2", "40", "addr-alice", 9, 2),
		payout("tx-3", "pay:3", "40", "addr-alice", 9, 4),
	}

	findings, err := Duplicates(records, 5*time.Minute)
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(findings) != 2 {
		t.Errorf("three payments two minutes apart are two suspicious pairs, got %d:\n%s",
			len(findings), render(findings))
	}
}

// Amounts are decimal, so 0.1+0.2 must be exactly 0.3 and an
// operation built from thirds must balance exactly.
func TestAmount_ArithmeticIsExactAndScaleInsensitive(t *testing.T) {
	total := NewSum()
	total.Add(MustParseAmount("0.1", usdt))
	total.Add(MustParseAmount("0.2", usdt))
	total.Add(MustParseAmount("-0.3", usdt))
	if !total.IsBalanced() {
		t.Errorf("0.1 + 0.2 - 0.3 = %s, want exactly zero", total.In(usdt))
	}

	// Trailing zeros must not make two equal quantities unequal.
	if !MustParseAmount("1.50", usdt).Equal(MustParseAmount("1.5", usdt)) {
		t.Error("1.50 and 1.5 are the same quantity")
	}
	if !MustParseAmount("-0", usdt).IsZero() {
		t.Error("-0 is zero")
	}
}

// Replay must not be confused by two operations at the same instant.
// The tie-break is the id, so the order is defined — but the finding should
// name the operation that actually broke the balance under that order.
func TestReplay_TwoOperationsAtOneInstant_AreOrderedByID(t *testing.T) {
	same := func(id string) Operation {
		o := paymentWithoutHold(id, "60", 10)
		o.At = at(2, 10, 0)
		return o
	}
	operations := []Operation{funded(), same("op-2-pay"), same("op-3-pay")}

	findings, err := Replay(operations, openingZero(userAccount))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Operation != "op-3-pay" {
		t.Errorf("under the id tie-break the second payment breaks it, got %q", findings[0].Operation)
	}
}

// An operation whose legs are all zero is degenerate but legal. No
// check should report it, and none should divide by it.
func TestChecks_ZeroValuedOperation_IsNobodysFinding(t *testing.T) {
	operation := Operation{
		ID: "op-zero", Kind: OpTransfer, At: fixtureTime, Ref: "zero:1",
		Legs: []Leg{
			leg("l1", userAccount, "0", usdt, RolePrincipal),
			leg("l2", chainAccount, "0", usdt, RolePrincipal),
		},
	}

	for name, run := range map[string]func() ([]Finding, error){
		"conserve":  func() ([]Finding, error) { return Conserve([]Operation{operation}) },
		"reversals": func() ([]Finding, error) { return Reversals([]Operation{operation}) },
		"replay": func() ([]Finding, error) {
			return Replay([]Operation{operation}, openingZero(userAccount))
		},
	} {
		findings, err := run()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(findings) != 0 {
			t.Errorf("%s reported a zero-valued operation:\n%s", name, render(findings))
		}
	}
}

// Boundary indexes operations by Ref. Two external records sharing
// one Ref both count as internal if the operation is internal — but the
// operation is one movement. Does the gap still reconcile?
func TestBoundary_OneInternalMovementRecordedTwice_IsNotAnExplanation(t *testing.T) {
	second := sweepRecord()
	second.ID = "tx-sweep-again"

	findings, err := Boundary(
		[]Operation{depositCrossing(), sweepInternal()},
		[]ExternalRecord{depositRecord(), sweepRecord(), second},
	)
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	for _, f := range findings {
		if f.Mode == ModeInternalMovement {
			t.Errorf("one internal movement recorded twice outside is not explained by it; "+
				"the duplicate is the finding:\n%s", f)
		}
	}
}

// Findings must survive being compared by content. Two runs over
// inputs that differ only in the order of the legs inside an operation should
// produce the same finding.
func TestConserve_LegOrder_DoesNotChangeTheFinding(t *testing.T) {
	forwards := withdrawalFeeInTRX()
	backwards := withdrawalFeeInTRX()
	backwards.Legs = []Leg{backwards.Legs[2], backwards.Legs[0], backwards.Legs[1]}

	a, err := Conserve([]Operation{forwards})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}
	b, err := Conserve([]Operation{backwards})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}
	if len(a) != 1 || len(b) != 1 || a[0].Arithmetic != b[0].Arithmetic {
		t.Errorf("leg order changed the finding:\n%s\n---\n%s", render(a), render(b))
	}
}

// Every exported entry point should refuse a nil slice without
// panicking, and say nothing rather than something.
func TestChecks_EmptyInput_InventsNothing(t *testing.T) {
	calls := map[string]func() ([]Finding, error){
		"Conserve":       func() ([]Finding, error) { return Conserve(nil) },
		"FeeConventions": func() ([]Finding, error) { return FeeConventions(nil) },
		"FeeEstimates":   func() ([]Finding, error) { return FeeEstimates(nil, nil) },
		"Replay":         func() ([]Finding, error) { return Replay(nil, nil) },
		"Daily":          func() ([]Finding, error) { return Daily(nil, time.UTC) },
		"Rates":          func() ([]Finding, error) { return Rates(nil, nil, usdt, nil) },
		"Boundary":       func() ([]Finding, error) { return Boundary(nil, nil) },
		"Batches":        func() ([]Finding, error) { return Batches(nil, nil) },
		"Finality":       func() ([]Finding, error) { return Finality(nil, nil, nil) },
		"Duplicates":     func() ([]Finding, error) { return Duplicates(nil, time.Minute) },
		"Reversals":      func() ([]Finding, error) { return Reversals(nil) },
		"Configure":      func() ([]Finding, error) { return Configure(nil, nil, nil) },
	}
	for name, call := range calls {
		findings, err := call()
		if err != nil {
			t.Errorf("%s on empty input returned an error: %v", name, err)
		}
		if len(findings) != 0 {
			t.Errorf("%s invented %d findings from nothing", name, len(findings))
		}
	}
}

// A finding's Summary and Arithmetic are read by people. Neither
// should ever come out with a dangling "%!" verb from a bad format string.
func TestFinding_RenderingHasNoFormattingAccidents(t *testing.T) {
	report, err := loadInput(t, "testdata/example.json").Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, f := range report.Findings {
		for _, text := range []string{f.Summary, f.Arithmetic, f.String()} {
			if strings.Contains(text, "%!") || strings.Contains(text, "<nil>") {
				t.Errorf("mode %s renders badly: %q", f.Mode.Number(), text)
			}
		}
	}
}
