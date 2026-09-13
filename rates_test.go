package reconcile

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func quote(rate string, day, hour int) Quote {
	return Quote{
		Base:    btc,
		Counter: usdt,
		Rate:    decimal.RequireFromString(rate),
		At:      at(day, hour, 0),
	}
}

// tradedBTC buys BTC, quoted at one moment, executed at another, settled at a
// third.
func tradedBTC(id string, ordered, executed, settled int) Operation {
	return Operation{
		ID: id, Kind: OpTrade, At: at(2, ordered, 0),
		Timeline: &Timeline{
			Ordered:  at(2, ordered, 0),
			Executed: at(2, executed, 0),
			Settled:  at(2, settled, 0),
		},
		Legs: []Leg{
			leg("l1", userAccount, "-2", btc, RolePrincipal),
			leg("l2", treasuryAccount, "2", btc, RolePrincipal),
		},
	}
}

var steadyMarket = []Quote{quote("60000", 2, 8)}

var movingMarket = []Quote{
	quote("60000", 2, 8),
	quote("61000", 2, 10),
	quote("64000", 2, 14),
}

func TestRates_RateUnchangedThroughout_ReportsNothing(t *testing.T) {
	findings, err := Rates([]Operation{tradedBTC("op-a", 9, 11, 15)}, steadyMarket, usdt, nil)
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// Mode 07. Two BTC, quoted at 60000, executed after the rate moved to 61000,
// settled after it reached 64000. Three reports of one transaction, each
// defensible, none of them agreeing.
func TestRates_RateMovedBetweenMoments_ReportsAllThreeFigures(t *testing.T) {
	findings, err := Rates([]Operation{tradedBTC("op-a", 9, 11, 15)}, movingMarket, usdt, nil)
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeRateAsOf {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeRateAsOf)
	}
	for _, want := range []string{
		"ordered", "executed", "settled",
		"x 60000 = 120000", "x 61000 = 122000", "x 64000 = 128000",
		"spread = 8000",
	} {
		if !strings.Contains(finding.Arithmetic, want) {
			t.Errorf("arithmetic does not contain %q: %q", want, finding.Arithmetic)
		}
	}
	if !strings.Contains(finding.Summary, "spread of 8000 USDT") {
		t.Errorf("summary = %q", finding.Summary)
	}
}

func TestRates_MovementWithinTolerance_ReportsNothing(t *testing.T) {
	// Ordered and executed only: 2 BTC at 60000 and at 61000, a spread of 2000.
	operation := tradedBTC("op-a", 9, 11, 11)

	findings, err := Rates([]Operation{operation}, movingMarket, usdt,
		Tolerance{usdt: MustParseAmount("2000", usdt)})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a spread of exactly the tolerance must not fire; got:\n%s", render(findings))
	}
}

// A quote that does not say when it applied cannot answer the only question
// this mode asks.
func TestRates_QuoteWithoutATimestamp_ReturnsError(t *testing.T) {
	undated := Quote{Base: btc, Counter: usdt, Rate: decimal.RequireFromString("60000")}

	_, err := Rates(nil, []Quote{undated}, usdt, nil)
	if err == nil {
		t.Fatal("an undated quote must be refused")
	}
	if !strings.Contains(err.Error(), "as of when") {
		t.Errorf("error = %v", err)
	}
}

func TestRates_NoCounterCurrency_ReturnsError(t *testing.T) {
	if _, err := Rates(nil, movingMarket, "", nil); err == nil {
		t.Fatal("a valuation needs a currency to be expressed in")
	}
}

// Only quotes at or before a moment are in force. Valuing an operation at a
// rate published afterwards would be hindsight dressed up as measurement.
func TestRates_MomentBeforeAnyQuote_IsNotValued(t *testing.T) {
	operation := tradedBTC("op-a", 7, 11, 15)

	findings, err := Rates([]Operation{operation}, movingMarket, usdt, nil)
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if strings.Contains(findings[0].Arithmetic, "ordered (") {
		t.Errorf("the ordered moment predates every quote and must not be valued: %q",
			findings[0].Arithmetic)
	}
}

func TestRates_OnlyOneMomentValuable_ReportsNothing(t *testing.T) {
	operation := Operation{
		ID: "op-a", Kind: OpTrade, At: at(2, 9, 0),
		Timeline: &Timeline{Settled: at(2, 15, 0)},
		Legs: []Leg{
			leg("l1", userAccount, "-2", btc, RolePrincipal),
			leg("l2", treasuryAccount, "2", btc, RolePrincipal),
		},
	}

	findings, err := Rates([]Operation{operation}, movingMarket, usdt, nil)
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("one figure cannot disagree with itself; got:\n%s", render(findings))
	}
}

func TestRates_OperationAlreadyInTheCounterCurrency_IsSkipped(t *testing.T) {
	operation := tradedBTC("op-a", 9, 11, 15)
	operation.Legs = []Leg{
		leg("l1", userAccount, "-100", usdt, RolePrincipal),
		leg("l2", treasuryAccount, "100", usdt, RolePrincipal),
	}

	findings, err := Rates([]Operation{operation}, movingMarket, usdt, nil)
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

func TestRates_NoTimeline_IsSkippedRatherThanGuessed(t *testing.T) {
	operation := tradedBTC("op-a", 9, 11, 15)
	operation.Timeline = nil

	findings, err := Rates([]Operation{operation}, movingMarket, usdt, nil)
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

func TestQuoteBook_InForce_ReturnsTheLatestQuoteAtOrBeforeTheMoment(t *testing.T) {
	book, err := newQuoteBook(movingMarket)
	if err != nil {
		t.Fatalf("newQuoteBook: %v", err)
	}
	key := pair{base: btc, counter: usdt}

	cases := []struct {
		moment time.Time
		want   string
		found  bool
	}{
		{at(2, 7, 0), "", false},
		{at(2, 8, 0), "60000", true},
		{at(2, 9, 59), "60000", true},
		{at(2, 10, 0), "61000", true},
		{at(3, 0, 0), "64000", true},
	}

	for _, c := range cases {
		got, found := book.inForce(key, c.moment)
		if found != c.found {
			t.Errorf("at %s: found = %v, want %v", c.moment, found, c.found)
			continue
		}
		if found && got.Rate.String() != c.want {
			t.Errorf("at %s: rate = %s, want %s", c.moment, got.Rate, c.want)
		}
	}
}
