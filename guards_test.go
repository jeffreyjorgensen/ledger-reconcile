package reconcile

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// What the checks refuse, and why each refusal is a decision rather than an
// omission. Three of these were written as questions during a read of the
// whole library and came back as defects; they are pinned here so that the
// answers cannot quietly change back.

// FeeConventions decides the majority by counting operations. With an
// equal split there is no majority — reporting one half as "the minority" is
// a coin toss dressed as a finding.
func TestFeeConventions_EvenSplit_NamesNoMinority(t *testing.T) {
	operations := []Operation{
		paymentFeeOnTop("op-a"), paymentFeeOnTop("op-b"),
		paymentFeeInside("op-c"), paymentFeeInside("op-d"),
	}
	findings, err := FeeConventions(operations)
	if err != nil {
		t.Fatalf("FeeConventions: %v", err)
	}
	if len(findings) != 4 {
		t.Fatalf("an even split implicates every payment, not one camp; got %d:\n%s",
			len(findings), render(findings))
	}
	for _, f := range findings {
		if strings.Contains(f.Summary, "neighbours") {
			t.Errorf("no camp is the minority here, so none may be described as one:\n%s", f)
		}
		if !strings.Contains(f.Summary, "split evenly") {
			t.Errorf("the summary must say the set is split rather than pick a side:\n%s", f)
		}
	}
}

// A tolerance given in the wrong currency must not silently become the
// tolerance for a currency it was never meant for.
func TestFeeEstimates_ToleranceInAnotherCurrency_IsNotBorrowed(t *testing.T) {
	operations := []Operation{payoutWithEstimate("op-a", "0.5", "0.6")}

	findings, err := FeeEstimates(operations, Tolerance{usdt: MustParseAmount("1000", usdt)})
	if err != nil {
		t.Fatalf("FeeEstimates: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("a USDT tolerance must not absorb a TRX drift; got %d findings", len(findings))
	}
}

// Finality judges records whose operation credits an internal balance.
// A deposit credited into a held balance is still a credit the customer was
// told about — or is it? Pin whichever answer the code gives, so that it is a
// decision rather than an accident.
func TestFinality_CreditIntoTheHeldBalance_IsTheCorrectHandling(t *testing.T) {
	operation := Operation{
		ID: "op-deposit-held", Kind: OpDeposit, At: at(2, 9, 0), Ref: "chain:tron:held",
		Legs: []Leg{
			leg("l1", chainAccount, "-100", usdt, RolePrincipal),
			{ID: "l2", Account: userAccount, Amount: MustParseAmount("100", usdt),
				Role: RolePrincipal, Part: BalanceHeld},
		},
	}
	record := ExternalRecord{
		ID: "tx-held", Ref: operation.Ref, Amount: MustParseAmount("100", usdt),
		Chain: "tron", Confirmations: 1,
	}

	findings, err := Finality([]Operation{operation}, []ExternalRecord{record}, reorgingChain)
	if err != nil {
		t.Fatalf("Finality: %v", err)
	}
	// Value parked in the held part is exactly what "shows to the customer as
	// waiting" means in the article. It is not spendable, so crediting it
	// shallow is not the mode.
	if len(findings) != 0 {
		t.Errorf("value held pending confirmation is the correct handling, not the mode:\n%s",
			render(findings))
	}
}

// Configure is given the balances as a list. Two balances for one account
// and currency are contradictory input; the checker must not quietly average
// or take the last.
func TestConfigure_TwoRowsForOneAccount_AreEachJudged(t *testing.T) {
	balances := []Balance{balanceOf(usdt, "500", ""), balanceOf(usdt, "1", "")}

	findings, err := Configure(balances, workingRules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// Both rows are judged on their own terms. The library is not told these
	// are the same balance twice rather than two snapshots, and inventing that
	// reading would be a guess; judging each and reporting what each implies
	// is the answer it can defend.
	if len(findings) != 1 {
		t.Errorf("expected the row that cannot be withdrawn to be reported, got %d:\n%s",
			len(findings), render(findings))
	}
}

// Daily groups by the pair of dates. An operation on a day where the zone
// and UTC agree, next to one where they do not, must not drag the agreeing one
// into the finding.
func TestDaily_OperationsAwayFromTheBoundary_AreNotDraggedIn(t *testing.T) {
	findings, err := Daily([]Operation{
		nearMidnight("op-late"),
		middayOperation("op-mid-1"),
		middayOperation("op-mid-2"),
	}, berlin)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(findings))
	}
	if strings.Contains(findings[0].Arithmetic, "op-mid") {
		t.Errorf("an operation that belongs to one date in both readings is not in the gap: %q",
			findings[0].Arithmetic)
	}
}

// Rates values an operation from its principal currency. A quote for the
// reversed pair (USDT/BTC rather than BTC/USDT) must not be picked up as if it
// were the rate asked for.
func TestRates_QuoteForTheReversedPair_IsNotUsed(t *testing.T) {
	reversed := []Quote{{
		Base: usdt, Counter: btc,
		Rate: decimal.RequireFromString("0.0000166"), At: at(2, 8, 0),
	}}

	findings, err := Rates([]Operation{tradedBTC("op-a", 9, 11, 15)}, reversed, usdt, nil)
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("a USDT/BTC quote does not answer what 2 BTC is worth in USDT:\n%s",
			render(findings))
	}
}

// A negative rate is nonsense and would silently invert a valuation.
func TestRates_RateAtOrBelowZero_IsRefused(t *testing.T) {
	bad := []Quote{
		{Base: btc, Counter: usdt, Rate: decimal.RequireFromString("-60000"), At: at(2, 8, 0)},
		{Base: btc, Counter: usdt, Rate: decimal.RequireFromString("61000"), At: at(2, 10, 0)},
	}

	_, err := Rates([]Operation{tradedBTC("op-a", 9, 11, 15)}, bad, usdt, nil)
	if err == nil {
		t.Fatal("a negative rate is not a price; it must be refused rather than used")
	}
	if !strings.Contains(err.Error(), "not a price") {
		t.Errorf("error = %v", err)
	}

	zero := []Quote{{Base: btc, Counter: usdt, Rate: decimal.Zero, At: at(2, 8, 0)}}
	if _, err := Rates(nil, zero, usdt, nil); err == nil {
		t.Error("a rate of zero makes everything worthless; it must be refused too")
	}
}

// The JSON form of a Finding must round-trip, since callers are invited to
// consume the -json output.
func TestFinding_SurvivesJSONWithItsEvidence(t *testing.T) {
	findings, err := Conserve([]Operation{withdrawalFeeInTRX()})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}

	encoded, err := json.Marshal(findings[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Finding
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Mode != findings[0].Mode || back.Arithmetic != findings[0].Arithmetic {
		t.Errorf("finding did not survive JSON:\n%+v\n%+v", findings[0], back)
	}
	if len(back.Evidence) != len(findings[0].Evidence) {
		t.Errorf("evidence lost: %d became %d", len(findings[0].Evidence), len(back.Evidence))
	}
	if len(back.Evidence) > 0 && !back.Evidence[0].Amount.Equal(findings[0].Evidence[0].Amount) {
		t.Error("an amount inside the evidence did not survive JSON")
	}
}

// Operations arriving with no timestamp at all. Replay orders by time, so
// a zero time is "the beginning" — which is a decision, not an accident, and
// should be visible.
func TestReplay_UndatedOperation_IsRefusedNotSortedToTheFront(t *testing.T) {
	undated := paymentWithoutHold("op-undated", "60", 10)
	undated.At = time.Time{}

	_, err := Replay([]Operation{funded(), undated}, openingZero(userAccount))
	if err == nil {
		t.Fatal("the replay is entirely about order; an undated operation must be refused, " +
			"not silently sorted to the front")
	}
	if !strings.Contains(err.Error(), "carries no timestamp") {
		t.Errorf("error = %v", err)
	}

	// The same holds for the day boundary: a zero time has no date in either
	// reading and would be filed under the year one.
	if _, err := Daily([]Operation{undated}, berlin); err == nil {
		t.Error("an undated operation belongs to no day and must be refused")
	}
}

// Batches groups by BatchRef. An operation claiming a batch that a record
// also claims by Ref is the normal case; but two batches whose refs differ
// only by case are two batches, not one.
func TestBatches_ReferencesAreComparedExactly(t *testing.T) {
	a := batched("op-1", "40", "tx-batch")
	b := batched("op-2", "70", "TX-BATCH")
	record := ExternalRecord{ID: "tx", Ref: "tx-batch",
		Amount: MustParseAmount("-40", usdt), At: at(2, 9, 5)}

	findings, err := Batches([]Operation{a, b}, []ExternalRecord{record})
	if err != nil {
		t.Fatalf("Batches: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("tx-batch settles exactly; TX-BATCH has no record and is the only finding; got %d:\n%s",
			len(findings), render(findings))
	}
}
