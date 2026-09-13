package reconcile

import (
	"strings"
	"testing"
)

func balanceOf(currency Currency, available, frozen string) Balance {
	balance := Balance{
		Account:   userAccount,
		Available: MustParseAmount(available, currency),
		Held:      zeroAmount(currency),
	}
	if frozen != "" {
		balance.Frozen = MustParseAmount(frozen, currency)
	}
	return balance
}

var workingRules = AssetRules{
	usdt: {
		Networks:          []string{"tron", "ethereum"},
		MinimumWithdrawal: MustParseAmount("10", usdt),
	},
}

var operatorNetworks = []string{"tron", "bitcoin"}

func TestConfigure_AssetThatCanLeaveAndEnoughToSend_ReportsNothing(t *testing.T) {
	findings, err := Configure([]Balance{balanceOf(usdt, "500", "")}, workingRules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", render(findings))
	}
}

// Mode 03 in its plainest form: the balance can be credited and there is
// nowhere for it to go. No arithmetic over movements would ever reveal this.
func TestConfigure_AssetWithNoNetwork_ReportsMode03(t *testing.T) {
	rules := AssetRules{btc: {MinimumWithdrawal: MustParseAmount("0.001", btc)}}

	findings, err := Configure([]Balance{balanceOf(btc, "2", "")}, rules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if findings[0].Mode != ModeAssetWithoutNetwork {
		t.Errorf("mode = %s, want %s", findings[0].Mode, ModeAssetWithoutNetwork)
	}
	if !strings.Contains(findings[0].Summary, "no network to leave on") {
		t.Errorf("summary = %q", findings[0].Summary)
	}
}

// The subtler form: the asset has networks, and the operator runs none of them.
func TestConfigure_AssetOnlyOnUnsupportedNetworks_ReportsMode03(t *testing.T) {
	rules := AssetRules{btc: {Networks: []string{"lightning"}}}

	findings, err := Configure([]Balance{balanceOf(btc, "2", "")}, rules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("an asset whose only network the operator does not run cannot be withdrawn")
	}
	if !strings.Contains(findings[0].Arithmetic, "supported [bitcoin tron]") {
		t.Errorf("arithmetic must show what is actually supported: %q", findings[0].Arithmetic)
	}
}

// A balance in an asset nothing describes is the state every one of these
// failures starts from, so it is reported rather than skipped.
func TestConfigure_AssetWithNoRuleAtAll_IsReported(t *testing.T) {
	findings, err := Configure([]Balance{balanceOf(trx, "100", "")}, workingRules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Summary, "no rule describes that asset") {
		t.Errorf("summary = %q", findings[0].Summary)
	}
}

// Mode 13. Everything here is working as configured, and the customer is
// looking at a number they cannot act on.
func TestConfigure_ReserveAndFreezeLeaveLessThanTheMinimum_ReportsMode13(t *testing.T) {
	rules := AssetRules{usdt: {
		Networks:          []string{"tron"},
		MinimumWithdrawal: MustParseAmount("10", usdt),
		Reserve:           MustParseAmount("5", usdt),
	}}

	findings, err := Configure([]Balance{balanceOf(usdt, "12", "4")}, rules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}

	finding := findings[0]
	if finding.Mode != ModeUnwithdrawable {
		t.Errorf("mode = %s, want %s", finding.Mode, ModeUnwithdrawable)
	}
	if !strings.Contains(finding.Arithmetic, "available 12 - frozen 4 - reserve 5 = 3, below the minimum of 10") {
		t.Errorf("arithmetic = %q", finding.Arithmetic)
	}
	// Every part of the gap is named, and the summary says none of them is a
	// missing entry — the wrong figure the reader would otherwise go looking
	// for does not exist.
	for _, want := range []string{"reserve", "freeze", "below the minimum", "none of them is a missing entry"} {
		if !strings.Contains(finding.Summary, want) {
			t.Errorf("summary does not name %q: %q", want, finding.Summary)
		}
	}
}

func TestConfigure_ReserveExceedsTheBalance_ReportsNothingWithdrawable(t *testing.T) {
	rules := AssetRules{usdt: {Networks: []string{"tron"}, Reserve: MustParseAmount("50", usdt)}}

	findings, err := Configure([]Balance{balanceOf(usdt, "20", "")}, rules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d:\n%s", len(findings), render(findings))
	}
	if !strings.Contains(findings[0].Arithmetic, "= 0, below the minimum of 0") {
		t.Errorf("a reserve larger than the balance leaves nothing, never a negative: %q",
			findings[0].Arithmetic)
	}
}

func TestConfigure_EmptyBalance_IsNotReported(t *testing.T) {
	findings, err := Configure([]Balance{balanceOf(usdt, "0", "")}, workingRules, operatorNetworks)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("an empty balance is nobody's complaint; got:\n%s", render(findings))
	}
}

func TestConfigure_MalformedBalance_ReturnsError(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Balance)
	}{
		{"no account", func(b *Balance) { b.Account.ID = "" }},
		{"unclassified account", func(b *Balance) { b.Account.Kind = AccountUnknown }},
		{"no currency", func(b *Balance) { b.Available = Amount{} }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			balance := balanceOf(usdt, "100", "")
			c.mutate(&balance)

			if _, err := Configure([]Balance{balance}, workingRules, operatorNetworks); err == nil {
				t.Fatalf("expected %s to be rejected", c.name)
			}
		})
	}
}
