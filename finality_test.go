package reconcile

import (
	"strings"
	"testing"
)

// depositOn is a deposit credited from one chain, with the record beside it.
func depositOn(chain string, confirmations int, final bool) (Operation, ExternalRecord) {
	operation := Operation{
		ID: "op-deposit-" + chain, Kind: OpDeposit, At: at(2, 9, 0),
		Ref: "chain:" + chain + ":deposit",
		Legs: []Leg{
			leg("l1", chainAccount, "-100", usdt, RolePrincipal),
			leg("l2", userAccount, "100", usdt, RolePrincipal),
		},
	}
	record := ExternalRecord{
		ID: "tx-" + chain, Ref: operation.Ref, Amount: MustParseAmount("100", usdt),
		At: at(2, 9, 0), Chain: chain, Confirmations: confirmations, Final: final,
	}
	return operation, record
}

var reorgingChain = FinalityRules{"tron": {Depth: 20}}

func TestFinality_CreditedDeepEnough_ReportsNothing(t *testing.T) {
	operation, record := depositOn("tron", 20, false)

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, reorgingChain)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

func TestFinality_CreditedTooShallow_ReportsMode09(t *testing.T) {
	operation, record := depositOn("tron", 3, false)

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, reorgingChain)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Mode != ModeFinality {
		t.Errorf("mode = %s, want %s", findings[0].Mode, ModeFinality)
	}
	if !strings.Contains(findings[0].Arithmetic, "3 < 20 confirmations required") {
		t.Errorf("arithmetic = %q", findings[0].Arithmetic)
	}
}

// The distinction the article's title makes. On a chain that publishes
// finality, a deep transaction may still be dropped and a shallow finalised one
// will not be, so depth is not an answer to the question being asked.
func TestFinality_DeepButNotFinalisedOnACheckpointChain_ReportsMode09(t *testing.T) {
	rules := FinalityRules{"ethereum": {ByCheckpoint: true}}
	operation, record := depositOn("ethereum", 64, false)

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, rules)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("64 confirmations are not finality on this chain; got:\n%s", render(findings))
	}
	if !strings.Contains(findings[0].Arithmetic, "not an answer to this question") {
		t.Errorf("arithmetic = %q", findings[0].Arithmetic)
	}
}

func TestFinality_FinalisedButShallowOnACheckpointChain_ReportsNothing(t *testing.T) {
	rules := FinalityRules{"ethereum": {ByCheckpoint: true}}
	operation, record := depositOn("ethereum", 2, true)

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, rules)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a finalised transaction is final however shallow; got:\n%s", render(findings))
	}
}

// No safe default exists. A depth chosen here would be a number nobody
// decided, and silence would be a claim of irreversibility this library has no
// basis for making.
func TestFinality_ChainWithNoRule_IsReportedRatherThanPassed(t *testing.T) {
	operation, record := depositOn("some-new-chain", 999, true)

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, reorgingChain)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("an unconfigured chain must be reported, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, "no finality rule configured") {
		t.Errorf("summary = %q", findings[0].Summary)
	}
}

func TestFinality_TransactionReorganisedAway_ReportsTheStandingCredit(t *testing.T) {
	operation, record := depositOn("tron", 20, false)
	record.Reorged = true

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, reorgingChain)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, "no longer carries the transaction that funded it") {
		t.Errorf("summary = %q", findings[0].Summary)
	}
}

// A withdrawal promises the customer nothing about irreversibility, so the
// mode does not apply to it.
func TestFinality_OperationThatCreditsNobody_IsSkipped(t *testing.T) {
	operation := paymentWithoutHold("op-pay", "40", 9)
	record := ExternalRecord{
		ID: "tx-pay", Ref: operation.Ref, Amount: MustParseAmount("-40", usdt),
		Chain: "tron", Confirmations: 1,
	}

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, reorgingChain)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

func TestFinality_RecordWithoutAChain_MakesNoClaimAndIsSkipped(t *testing.T) {
	operation, record := depositOn("tron", 1, false)
	record.Chain = ""

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, reorgingChain)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}
