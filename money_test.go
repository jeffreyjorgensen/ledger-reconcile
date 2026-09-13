package reconcile

import (
	"encoding/json"
	"testing"
)

// A parse that silently yields zero is the expensive failure, not a noisy one.
// Zero is a real answer everywhere money is involved — no minimum, no fee, no
// balance — so an unreadable field must not be allowed to impersonate it.
func TestParseAmount_Unreadable_ReturnsErrorNeverZero(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"words", "about a hundred"},
		{"thousands separator", "1,000.00"},
		{"currency glued on", "100USDT"},
		{"two dots", "1.0.0"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			amount, err := ParseAmount(c.text, usdt)
			if err == nil {
				t.Fatalf("ParseAmount(%q) returned %s with no error", c.text, amount)
			}
			if !amount.IsZero() || amount.Currency() != "" {
				t.Errorf("failed parse should yield the zero Amount, got %+v", amount)
			}
		})
	}
}

func TestParseAmount_NoCurrency_ReturnsError(t *testing.T) {
	if _, err := ParseAmount("1.00", ""); err == nil {
		t.Fatal("an amount without a currency must not be accepted")
	}
}

func TestAmountAdd_DifferentCurrencies_ReturnsError(t *testing.T) {
	hundredUSDT := MustParseAmount("100", usdt)
	someTRX := MustParseAmount("0.5", trx)

	if _, err := hundredUSDT.Add(someTRX); err == nil {
		t.Fatal("adding TRX to USDT must not be allowed: that is how mode 14 hides")
	}
	if _, err := hundredUSDT.Cmp(someTRX); err == nil {
		t.Fatal("comparing TRX with USDT must not be allowed")
	}
}

func TestAmountEqual_SameQuantityDifferentCurrency_IsNotEqual(t *testing.T) {
	if MustParseAmount("0", usdt).Equal(MustParseAmount("0", trx)) {
		t.Error("nothing in USDT and nothing in TRX are different facts")
	}
}

func TestSum_KeepsCurrenciesApart(t *testing.T) {
	total := NewSum()
	total.Add(MustParseAmount("-100", usdt))
	total.Add(MustParseAmount("100", usdt))
	total.Add(MustParseAmount("-0.5", trx))

	if !total.In(usdt).IsZero() {
		t.Errorf("USDT should balance, got %s", total.In(usdt))
	}
	if total.IsBalanced() {
		t.Error("the sum must not call itself balanced while TRX is short")
	}

	nonZero := total.NonZero()
	if len(nonZero) != 1 || nonZero[0].Currency() != trx {
		t.Fatalf("expected only TRX outstanding, got %+v", nonZero)
	}
	if nonZero[0].Value().String() != "-0.5" {
		t.Errorf("TRX residual = %s, want -0.5", nonZero[0].Value())
	}
}

func TestSumCurrencies_OrderIsStableAcrossRuns(t *testing.T) {
	build := func() []Currency {
		total := NewSum()
		for _, currency := range []Currency{"ZEC", "USDT", "BTC", "TRX", "ETH", "XMR", "SOL", "TON"} {
			total.Add(MustParseAmount("1", currency))
		}
		return total.Currencies()
	}

	want := build()
	for range 50 {
		if got := build(); !equalCurrencies(got, want) {
			t.Fatalf("currency order changed between runs: %v then %v", want, got)
		}
	}
}

// JSON amounts travel as strings. A JSON number has already been through a
// float by the time this code sees it, so accepting one would mean accepting a
// value that may already have been rounded.
func TestAmountUnmarshalJSON_QuantityAsNumber_IsRejected(t *testing.T) {
	var amount Amount
	if err := json.Unmarshal([]byte(`{"value":100.5,"currency":"USDT"}`), &amount); err == nil {
		t.Fatal("a numeric quantity must be rejected")
	}
}

func TestAmountJSON_RoundTrip_PreservesExactQuantity(t *testing.T) {
	// A quantity chosen because it is not representable in binary floating
	// point: a round trip through float64 would not return it unchanged.
	original := MustParseAmount("0.10000000000000000555", btc)

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Amount
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.Equal(original) {
		t.Errorf("round trip changed the amount: %s became %s", original, decoded)
	}
}

func equalCurrencies(a, b []Currency) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAmountSubAndNeg_ReverseDirectionWithoutTouchingTheCurrency(t *testing.T) {
	hundred := MustParseAmount("100", usdt)
	one := MustParseAmount("1", usdt)

	difference, err := hundred.Sub(one)
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if !difference.Equal(MustParseAmount("99", usdt)) {
		t.Errorf("100 - 1 = %s, want 99 USDT", difference)
	}

	negated := hundred.Neg()
	if negated.Sign() != -1 || negated.Currency() != usdt {
		t.Errorf("Neg() = %s, want -100 USDT", negated)
	}
	if !negated.Abs().Equal(hundred) {
		t.Errorf("Abs(Neg(100)) = %s, want 100 USDT", negated.Abs())
	}
}

func TestAmountSub_DifferentCurrencies_ReturnsError(t *testing.T) {
	if _, err := MustParseAmount("100", usdt).Sub(MustParseAmount("1", trx)); err == nil {
		t.Fatal("subtracting TRX from USDT must not be allowed")
	}
}

func TestAmountCmp_SameCurrency_OrdersByQuantity(t *testing.T) {
	less, err := MustParseAmount("1", usdt).Cmp(MustParseAmount("2", usdt))
	if err != nil {
		t.Fatalf("Cmp: %v", err)
	}
	if less != -1 {
		t.Errorf("1 vs 2 = %d, want -1", less)
	}
}

func TestAmountString_WithoutCurrency_PrintsTheQuantityAlone(t *testing.T) {
	if got := (Amount{}).String(); got != "0" {
		t.Errorf("zero Amount renders as %q, want \"0\"", got)
	}
}

func TestMustParseAmount_UnreadableLiteral_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustParseAmount must panic on an unreadable literal rather than yield zero")
		}
	}()
	MustParseAmount("not a number", usdt)
}

func TestSum_ZeroValueIsUsableWithoutNewSum(t *testing.T) {
	var total Sum
	total.Add(MustParseAmount("1", usdt))

	if !total.In(usdt).Equal(MustParseAmount("1", usdt)) {
		t.Errorf("zero-value Sum did not accumulate: %s", total.In(usdt))
	}
}

func TestSum_UntouchedCurrency_ReadsAsZeroInThatCurrency(t *testing.T) {
	total := NewSum()
	total.Add(MustParseAmount("1", usdt))

	untouched := total.In(btc)
	if !untouched.IsZero() || untouched.Currency() != btc {
		t.Errorf("In(BTC) on an untouched currency = %s, want 0 BTC", untouched)
	}
}
