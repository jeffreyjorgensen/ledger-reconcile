package reconcile

import (
	"strings"
	"testing"
)

// The negative case: the same money leaves, but the first payment reserves it
// before it goes, so the second check reads a figure that already accounts for
// the commitment.
func TestReplay_ValueReservedBeforeItLeaves_ReportsNothing(t *testing.T) {
	operations := []Operation{
		funded(),
		holdFor("op-2-hold", "60", 10),
		settleHeld("op-3-settle", "60", 11),
		paymentWithoutHold("op-4-pay", "40", 12),
	}

	findings, err := Replay(operations, openingZero(userAccount))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// Mode 04. Both payments are individually defensible: each was authorised
// against a balance of 100 and each is for 60. Nothing marked the first 60 as
// promised, so the second check read the same 100 the first did.
func TestReplay_TwoPaymentsAgainstOneBalance_ReportsMode04(t *testing.T) {
	operations := []Operation{
		funded(),
		paymentWithoutHold("op-2-pay", "60", 10),
		paymentWithoutHold("op-3-pay", "60", 11),
	}

	findings, err := Replay(operations, openingZero(userAccount))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeMissingHold {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeMissingHold)
	}
	if finding.Operation != "op-3-pay" {
		t.Errorf("the finding must name the operation that broke the balance, got %q", finding.Operation)
	}
	if !strings.Contains(finding.Arithmetic, "available = -20") {
		t.Errorf("arithmetic must state the balance it reached: %q", finding.Arithmetic)
	}
	// Naming what was reserved is the point. Nothing was, which is the mode.
	if !strings.Contains(finding.Arithmetic, "held = 0") {
		t.Errorf("arithmetic must state what was reserved: %q", finding.Arithmetic)
	}
	if !strings.Contains(finding.Arithmetic, "needed a hold of 20") {
		t.Errorf("arithmetic must say what would have covered it: %q", finding.Arithmetic)
	}
}

// Once a balance is negative every later operation on it is also negative.
// Reporting each would bury the one that caused it.
func TestReplay_BalanceStaysNegative_ReportsOnlyTheFirstBreach(t *testing.T) {
	operations := []Operation{
		funded(),
		paymentWithoutHold("op-2-pay", "60", 10),
		paymentWithoutHold("op-3-pay", "60", 11),
		paymentWithoutHold("op-4-pay", "60", 12),
		paymentWithoutHold("op-5-pay", "60", 13),
	}

	findings, err := Replay(operations, openingZero(userAccount))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected only the first breach, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Operation != "op-3-pay" {
		t.Errorf("reported %s, want the first breach op-3-pay", findings[0].Operation)
	}
}

// The counterparty account holds the mirror of what the ledger owes the
// outside world. Its position is routinely negative and that is not a defect.
func TestReplay_ExternalAccountGoesNegative_IsNotReported(t *testing.T) {
	findings, err := Replay([]Operation{funded()}, openingZero(userAccount))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("the external counterparty may hold a negative position; got:\n%s", render(findings))
	}
}

// The order is the whole signal, so it must come from the data and not from
// however the caller happened to pass it.
func TestReplay_InputOrderShuffled_ProducesTheSameFinding(t *testing.T) {
	forwards := []Operation{
		funded(),
		paymentWithoutHold("op-2-pay", "60", 10),
		paymentWithoutHold("op-3-pay", "60", 11),
	}
	backwards := []Operation{forwards[2], forwards[0], forwards[1]}

	first, err := Replay(forwards, openingZero(userAccount))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	second, err := Replay(backwards, openingZero(userAccount))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Arithmetic != second[0].Arithmetic {
		t.Fatalf("input order changed the finding:\n%s\n---\n%s", render(first), render(second))
	}
}

func TestReplay_MalformedOperation_ReturnsError(t *testing.T) {
	operation := funded()
	operation.Legs[0].Account.Kind = AccountUnknown

	if _, err := Replay([]Operation{operation}, openingZero(userAccount)); err == nil {
		t.Fatal("expected an error for an unclassified account")
	}
}

// openingZero states that each account began the window empty. Saying so is
// the point: the replay refuses to assume it.
func openingZero(accounts ...Account) []Balance {
	opening := make([]Balance, 0, len(accounts))
	for _, account := range accounts {
		opening = append(opening, Balance{
			Account:   account,
			Available: zeroAmount(usdt),
			Held:      zeroAmount(usdt),
		})
	}
	return opening
}

// Without a starting position, "went negative" and "started positive and we
// were not told" cannot be told apart. A sweep out of a treasury that was full
// yesterday would be reported as an overdraft.
func TestReplay_NoOpeningBalance_RefusesRatherThanAssumingZero(t *testing.T) {
	_, err := Replay([]Operation{funded()}, nil)
	if err == nil {
		t.Fatal("a replay with no stated starting position must refuse")
	}
	if !strings.Contains(err.Error(), "no opening balance given for acct-user-1 in USDT") {
		t.Errorf("the error must name what is missing: %v", err)
	}
	if !strings.Contains(err.Error(), "pass an explicit zero if it really began empty") {
		t.Errorf("the error must say how to proceed: %v", err)
	}
}

// The counterparty sits outside the ledger and has no starting position to
// state, so it is not demanded.
func TestReplay_ExternalAccountsNeedNoOpeningBalance(t *testing.T) {
	if _, err := Replay([]Operation{funded()}, openingZero(userAccount)); err != nil {
		t.Fatalf("Replay: %v", err)
	}
}

// An account that was already funded before the window pays out of what it
// had, and that is not an overdraft.
func TestReplay_AccountFundedBeforeTheWindow_IsNotReportedAsOverdrawn(t *testing.T) {
	opening := []Balance{{
		Account:   treasuryAccount,
		Available: MustParseAmount("500", usdt),
		Held:      zeroAmount(usdt),
	}}
	sweep := sweepInternal()

	findings, err := Replay([]Operation{sweep}, append(opening, Balance{
		Account: coldAccount, Available: zeroAmount(usdt), Held: zeroAmount(usdt),
	}))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a sweep out of a funded treasury is not an overdraft; got:\n%s", render(findings))
	}
}

// The same sweep out of an account that really was empty is one.
func TestReplay_AccountThatBeganEmpty_IsReportedWhenItPaysOut(t *testing.T) {
	opening := []Balance{
		{Account: treasuryAccount, Available: zeroAmount(usdt), Held: zeroAmount(usdt)},
		{Account: coldAccount, Available: zeroAmount(usdt), Held: zeroAmount(usdt)},
	}

	findings, err := Replay([]Operation{sweepInternal()}, opening)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Arithmetic, "available = -5") {
		t.Errorf("arithmetic = %q", findings[0].Arithmetic)
	}
}

// Held value carried in from before the window counts too: it is what a later
// check would have read.
func TestReplay_OpeningHeldIsCarriedIntoTheFinding(t *testing.T) {
	opening := []Balance{{
		Account:   userAccount,
		Available: zeroAmount(usdt),
		Held:      MustParseAmount("30", usdt),
	}}

	findings, err := Replay([]Operation{paymentWithoutHold("op-1-pay", "10", 9)}, opening)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Arithmetic, "held = 30") {
		t.Errorf("the reserved figure must be the one in force: %q", findings[0].Arithmetic)
	}
}
