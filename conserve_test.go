package reconcile

import (
	"reflect"
	"strings"
	"testing"
)

func TestConserve_BalancedOperations_ReportsNothing(t *testing.T) {
	operations := []Operation{
		withdrawalBalanced(),
		withdrawalFeeInTRXAccounted(),
		tradeBalanced(),
	}

	findings, err := Conserve(operations)
	if err != nil {
		t.Fatalf("Conserve returned an error on well-formed input: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %d:\n%s", len(findings), render(findings))
	}
}

func TestConserve_FeePaidInAnotherAsset_ReportsMode14(t *testing.T) {
	findings, err := Conserve([]Operation{withdrawalFeeInTRX()})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeFeeInOtherAsset {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeFeeInOtherAsset)
	}
	if finding.Currency != trx {
		t.Errorf("currency = %s, want %s", finding.Currency, trx)
	}
	if finding.Operation != "op-withdraw-fee-trx" {
		t.Errorf("operation = %q, want op-withdraw-fee-trx", finding.Operation)
	}

	// The arithmetic has to show both halves: TRX short, USDT square. Without
	// the second half the finding does not explain why nobody noticed.
	if !strings.Contains(finding.Arithmetic, "sum(legs in TRX) = -0.5") {
		t.Errorf("arithmetic does not state the TRX residual: %q", finding.Arithmetic)
	}
	if !strings.Contains(finding.Arithmetic, "sum(legs in USDT) = 0") {
		t.Errorf("arithmetic does not state that USDT balances: %q", finding.Arithmetic)
	}
	if len(finding.Evidence) != 1 || finding.Evidence[0].LegID != "l3" {
		t.Errorf("evidence should name the unaccounted fee leg, got %+v", finding.Evidence)
	}
	if finding.Article != "https://jeffreyjorgensen.dev/teardown#i14" {
		t.Errorf("article link = %q", finding.Article)
	}
}

// This is the test that justifies summing per currency rather than globally.
// The operation is short 0.0025 BTC while USDT balances exactly. A checker
// holding one running total would have to add a BTC quantity to a USDT one to
// see anything, and whatever it then reported would be meaningless.
func TestConserve_TradeMissingCounterLeg_ReportsOnlyTheShortCurrency(t *testing.T) {
	findings, err := Conserve([]Operation{tradeMissingOneLeg()})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Currency != btc {
		t.Errorf("currency = %s, want %s — USDT balances and must not be reported",
			findings[0].Currency, btc)
	}
	if findings[0].Mode != ModeUnbalanced {
		t.Errorf("mode = %s, want %s: no fee leg explains this residual",
			findings[0].Mode, ModeUnbalanced)
	}
}

func TestConserve_UnclassifiedAccount_ReturnsErrorRatherThanGuessing(t *testing.T) {
	operation := withdrawalBalanced()
	operation.Legs[0].Account.Kind = AccountUnknown

	if _, err := Conserve([]Operation{operation}); err == nil {
		t.Fatal("expected an error for an unclassified account, got none")
	}
}

func TestConserve_SameInputTwice_ProducesIdenticalFindings(t *testing.T) {
	operations := []Operation{
		tradeMissingOneLeg(),
		withdrawalFeeInTRX(),
		withdrawalBalanced(),
	}

	first, err := Conserve(operations)
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}
	for range 20 {
		again, err := Conserve(operations)
		if err != nil {
			t.Fatalf("Conserve: %v", err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("findings differ between runs on identical input:\n%s\n---\n%s",
				render(first), render(again))
		}
	}
}

func TestMode_ReferenceRewritten_IsNotClaimedAsCovered(t *testing.T) {
	if ModeReferenceRewritten.Covered() {
		t.Error("mode 08 must not report itself as covered")
	}
	for mode := ModeFeeConvention; mode <= ModeFeeInOtherAsset; mode++ {
		if mode == ModeReferenceRewritten {
			continue
		}
		if !mode.Covered() {
			t.Errorf("mode %s reports itself as uncovered", mode.Number())
		}
		if mode.Title() == "unknown mode" {
			t.Errorf("mode %s has no title", mode.Number())
		}
	}
}

// render prints findings for a failure message.
func render(findings []Finding) string {
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, finding.String())
	}
	return strings.Join(lines, "\n")
}
