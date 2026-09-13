package reconcile

import (
	"strings"
	"testing"
	"time"
)

func TestDaily_NoZoneGiven_ReturnsErrorRatherThanAssumingUTC(t *testing.T) {
	_, err := Daily([]Operation{middayOperation("op-a")}, nil)
	if err == nil {
		t.Fatal("assuming UTC would silence this check exactly when it matters")
	}
	if !strings.Contains(err.Error(), "cannot assume UTC") {
		t.Errorf("error = %v", err)
	}
}

func TestDaily_EverythingFarFromMidnight_ReportsNothing(t *testing.T) {
	findings, err := Daily([]Operation{middayOperation("op-a"), middayOperation("op-b")}, berlin)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// Mode 05. 23:30 UTC on the 1st is 00:30 on the 2nd in Berlin. Both reports
// are built honestly from the same data and they disagree by exactly this
// operation.
func TestDaily_OperationAcrossMidnight_ReportsMode05WithBothDates(t *testing.T) {
	findings, err := Daily([]Operation{middayOperation("op-mid"), nearMidnight("op-late")}, berlin)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeDayBoundary {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeDayBoundary)
	}
	if finding.Currency != usdt {
		t.Errorf("currency = %s, want %s", finding.Currency, usdt)
	}
	for _, want := range []string{"2026-01-02", "2026-01-01", "Europe/Berlin", "op-late"} {
		if !strings.Contains(finding.Arithmetic, want) {
			t.Errorf("arithmetic does not mention %q: %q", want, finding.Arithmetic)
		}
	}
	if !strings.Contains(finding.Summary, "differ by 40 USDT") {
		t.Errorf("summary must state the size of the disagreement: %q", finding.Summary)
	}
	if finding.Operation != "op-late" {
		t.Errorf("operation = %q, want op-late", finding.Operation)
	}
}

func TestDaily_SeveralOperationsAcrossTheSameMidnight_AreReportedTogether(t *testing.T) {
	second := nearMidnight("op-late-2")
	findings, err := Daily([]Operation{nearMidnight("op-late-1"), second}, berlin)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding for one boundary, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, "2 operation(s)") {
		t.Errorf("summary should count both: %q", findings[0].Summary)
	}
	if !strings.Contains(findings[0].Summary, "differ by 80 USDT") {
		t.Errorf("summary should total both: %q", findings[0].Summary)
	}
}

// An internal transfer belongs to a different date in the two interpretations
// just as much, but it moves nothing across the boundary, so no report of
// deposits and withdrawals can disagree about it.
func TestDaily_InternalMovementAcrossMidnight_MovesNoReportedValue(t *testing.T) {
	sweep := sweepInternal()
	sweep.At = at(1, 23, 45)

	findings, err := Daily([]Operation{sweep}, berlin)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

func TestDaily_ZoneEastOfUTC_PutsTheOperationOnTheLaterDate(t *testing.T) {
	tokyo := time.FixedZone("Asia/Tokyo", 9*60*60)

	findings, err := Daily([]Operation{nearMidnight("op-late")}, tokyo)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Arithmetic, "2026-01-02 in Asia/Tokyo = 2026-01-01 in UTC") {
		t.Errorf("arithmetic = %q", findings[0].Arithmetic)
	}
}
