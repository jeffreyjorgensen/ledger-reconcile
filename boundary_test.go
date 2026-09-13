package reconcile

import (
	"strings"
	"testing"
)

func TestBoundary_SidesAgree_ReportsNothing(t *testing.T) {
	findings, err := Boundary(
		[]Operation{depositCrossing(), sweepInternal()},
		[]ExternalRecord{depositRecord()},
	)
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// Every entry on both sides is correct. The sweep really happened on the chain
// and really moved nothing across the ledger's boundary, and counting it makes
// the equation fail by exactly its amount.
func TestBoundary_SweepCountedAsMovement_ReportsMode11AndNamesIt(t *testing.T) {
	findings, err := Boundary(
		[]Operation{depositCrossing(), sweepInternal()},
		[]ExternalRecord{depositRecord(), sweepRecord()},
	)
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeInternalMovement {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeInternalMovement)
	}
	if finding.Operation != "op-sweep" {
		t.Errorf("the finding must name the movement that explains the gap, got %q", finding.Operation)
	}
	if !strings.Contains(finding.Arithmetic, "external 105 - crossing 100 = 5") {
		t.Errorf("arithmetic = %q", finding.Arithmetic)
	}
	if !strings.Contains(finding.Summary, "never crossed the boundary") {
		t.Errorf("summary = %q", finding.Summary)
	}
}

// A gap that internal movements do not account for must not be dressed up as
// mode 11. Explaining away a discrepancy the input does not explain is worse
// than reporting it bare.
func TestBoundary_GapNotExplainedByInternalMovements_IsReportedAsItStands(t *testing.T) {
	stray := ExternalRecord{ID: "tx-stray", Ref: "chain:unknown:1", Amount: MustParseAmount("7", usdt)}

	findings, err := Boundary(
		[]Operation{depositCrossing(), sweepInternal()},
		[]ExternalRecord{depositRecord(), sweepRecord(), stray},
	)
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Mode != ModeUnbalanced {
		t.Errorf("mode = %s, want %s: internal movements explain only part of this gap",
			findings[0].Mode, ModeUnbalanced)
	}
	if !strings.Contains(findings[0].Arithmetic, "internal movements explain only 5") {
		t.Errorf("arithmetic must say how much was explained: %q", findings[0].Arithmetic)
	}
}

func TestBoundary_DuplicateReference_ReturnsErrorRatherThanPickingOne(t *testing.T) {
	first := depositCrossing()
	second := depositCrossing()
	second.ID = "op-deposit-again"

	_, err := Boundary([]Operation{first, second}, []ExternalRecord{depositRecord()})
	if err == nil {
		t.Fatal("two operations sharing a reference must be refused, not resolved arbitrarily")
	}
	if !strings.Contains(err.Error(), "share the reference") {
		t.Errorf("error = %v", err)
	}
}

func TestBoundary_SeveralInternalMovements_AreCountedTogether(t *testing.T) {
	second := sweepInternal()
	second.ID = "op-sweep-2"
	second.Ref = "chain:sweep:2"
	secondRecord := sweepRecord()
	secondRecord.ID = "tx-sweep-2"
	secondRecord.Ref = "chain:sweep:2"

	findings, err := Boundary(
		[]Operation{depositCrossing(), sweepInternal(), second},
		[]ExternalRecord{depositRecord(), sweepRecord(), secondRecord},
	)
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, "2 movement(s)") {
		t.Errorf("summary should count both movements: %q", findings[0].Summary)
	}
	if !strings.Contains(findings[0].Operation, "+1 more") {
		t.Errorf("operation field should signal there is more than one: %q", findings[0].Operation)
	}
}

func TestBoundary_SameInputTwice_ProducesIdenticalFindings(t *testing.T) {
	operations := []Operation{depositCrossing(), sweepInternal()}
	records := []ExternalRecord{depositRecord(), sweepRecord()}

	first, err := Boundary(operations, records)
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	for range 20 {
		again, err := Boundary(operations, records)
		if err != nil {
			t.Fatalf("Boundary: %v", err)
		}
		if len(again) != len(first) || again[0].Arithmetic != first[0].Arithmetic {
			t.Fatalf("findings differ between runs:\n%s\n---\n%s", render(first), render(again))
		}
	}
}
