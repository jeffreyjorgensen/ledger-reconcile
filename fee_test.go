package reconcile

import (
	"strings"
	"testing"
)

func TestFeeConventions_OneConventionThroughout_ReportsNothing(t *testing.T) {
	for _, set := range [][]Operation{
		{paymentFeeOnTop("op-a"), paymentFeeOnTop("op-b"), paymentFeeOnTop("op-c")},
		{paymentFeeInside("op-a"), paymentFeeInside("op-b")},
	} {
		findings, err := FeeConventions(set)
		if err != nil {
			t.Fatalf("FeeConventions: %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("a consistent convention is a decision, not a defect; got:\n%s", render(findings))
		}
	}
}

// The identical postings appear in both camps. Only the declared amount says
// which convention a payment followed, which is why the detector refuses to
// run without it.
func TestFeeConventions_BothConventionsInOneSet_ReportsTheMinority(t *testing.T) {
	operations := []Operation{
		paymentFeeOnTop("op-a"),
		paymentFeeOnTop("op-b"),
		paymentFeeOnTop("op-c"),
		paymentFeeInside("op-odd"),
	}

	findings, err := FeeConventions(operations)
	if err != nil {
		t.Fatalf("FeeConventions: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected the single minority payment, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Operation != "op-odd" {
		t.Errorf("reported %s, want op-odd", findings[0].Operation)
	}
	if findings[0].Mode != ModeFeeConvention {
		t.Errorf("mode = %s, want %s", findings[0].Mode, ModeFeeConvention)
	}
	if !strings.Contains(findings[0].Summary, "3 of its neighbours") {
		t.Errorf("summary does not weigh the minority against the majority: %q", findings[0].Summary)
	}
}

func TestFeeConventions_DeclaredMatchesNeither_IsReportedOnItsOwn(t *testing.T) {
	findings, err := FeeConventions([]Operation{paymentDeclaringNeither("op-odd")})
	if err != nil {
		t.Fatalf("FeeConventions: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if !strings.Contains(finding.Arithmetic, "declared = 95") {
		t.Errorf("arithmetic does not state the declared figure: %q", finding.Arithmetic)
	}
	if !strings.Contains(finding.Arithmetic, "debited = 100") {
		t.Errorf("arithmetic does not state what was debited: %q", finding.Arithmetic)
	}
	if !strings.Contains(finding.Arithmetic, "sent = 99") {
		t.Errorf("arithmetic does not state what was sent: %q", finding.Arithmetic)
	}
}

func TestFeeConventions_NoDeclaredAmount_IsSkippedRatherThanGuessed(t *testing.T) {
	operation := paymentFeeOnTop("op-a")
	operation.Declared = nil

	findings, err := FeeConventions([]Operation{operation})
	if err != nil {
		t.Fatalf("FeeConventions: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("without a declared amount there is nothing to compare against; got:\n%s", render(findings))
	}
}

func TestFeeConventions_NoFeeCharged_IsNotPutInEitherCamp(t *testing.T) {
	operation := payment("op-free", "100")
	operation.Legs = []Leg{
		leg("l1", userAccount, "-100", usdt, RolePrincipal),
		leg("l2", chainAccount, "100", usdt, RolePrincipal),
	}

	findings, err := FeeConventions([]Operation{operation, paymentFeeOnTop("op-a")})
	if err != nil {
		t.Fatalf("FeeConventions: %v", err)
	}
	for _, finding := range findings {
		if finding.Operation == "op-free" {
			t.Errorf("a payment with no fee follows neither convention and must not be reported: %s", finding)
		}
	}
}

func TestFeeEstimates_ChargedWhatWasQuoted_ReportsNothing(t *testing.T) {
	findings, err := FeeEstimates([]Operation{payoutWithEstimate("op-a", "0.5", "0.5")}, nil)
	if err != nil {
		t.Fatalf("FeeEstimates: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected nothing, got:\n%s", render(findings))
	}
}

func TestFeeEstimates_ChargedMoreThanQuoted_ReportsMode12(t *testing.T) {
	findings, err := FeeEstimates([]Operation{payoutWithEstimate("op-a", "0.5", "1.25")}, nil)
	if err != nil {
		t.Fatalf("FeeEstimates: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeFeeEstimate {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeFeeEstimate)
	}
	if finding.Currency != trx {
		t.Errorf("currency = %s, want %s", finding.Currency, trx)
	}
	if !strings.Contains(finding.Arithmetic, "actual 1.25 - estimate 0.5 = 0.75") {
		t.Errorf("arithmetic = %q", finding.Arithmetic)
	}
}

// An unconfigured tolerance has to mean zero, not infinity. A detector that
// went quiet because nobody configured it would be worse than no detector: it
// would look like a passing check.
func TestFeeEstimates_ToleranceAbsorbsDriftOnlyUpToItsValue(t *testing.T) {
	operations := []Operation{payoutWithEstimate("op-a", "0.5", "0.6")}

	within, err := FeeEstimates(operations, Tolerance{trx: MustParseAmount("0.1", trx)})
	if err != nil {
		t.Fatalf("FeeEstimates: %v", err)
	}
	if len(within) != 0 {
		t.Errorf("drift of exactly the tolerance must not fire; got:\n%s", render(within))
	}

	outside, err := FeeEstimates(operations, Tolerance{trx: MustParseAmount("0.05", trx)})
	if err != nil {
		t.Fatalf("FeeEstimates: %v", err)
	}
	if len(outside) != 1 {
		t.Errorf("drift beyond the tolerance must fire; got %d findings", len(outside))
	}

	unconfigured, err := FeeEstimates(operations, Tolerance{})
	if err != nil {
		t.Fatalf("FeeEstimates: %v", err)
	}
	if len(unconfigured) != 1 {
		t.Error("an unconfigured tolerance must mean zero, not silence")
	}
}

func TestFeeEstimates_ChargedLessThanQuoted_IsAlsoReported(t *testing.T) {
	findings, err := FeeEstimates([]Operation{payoutWithEstimate("op-a", "1.5", "0.25")}, nil)
	if err != nil {
		t.Fatalf("FeeEstimates: %v", err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Summary, "less than quoted") {
		t.Fatalf("a payout that cost less than quoted is still a changed cost; got:\n%s", render(findings))
	}
}

func TestFeeEstimates_MalformedOperation_ReturnsError(t *testing.T) {
	operation := payoutWithEstimate("op-a", "0.5", "0.5")
	operation.Legs[0].Account.Kind = AccountUnknown

	if _, err := FeeEstimates([]Operation{operation}, nil); err == nil {
		t.Fatal("expected an error for an unclassified account")
	}
	if _, err := FeeConventions([]Operation{operation}); err == nil {
		t.Fatal("expected an error for an unclassified account")
	}
}
