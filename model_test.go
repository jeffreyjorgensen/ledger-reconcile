package reconcile

import (
	"strings"
	"testing"
)

func TestAccountKind_InternalAndExternalAreSeparated(t *testing.T) {
	cases := []struct {
		kind     AccountKind
		internal bool
		name     string
	}{
		{AccountUser, true, "user"},
		{AccountFee, true, "fee"},
		{AccountSystem, true, "system"},
		{AccountExternal, false, "external"},
		{AccountUnknown, false, "unclassified"},
	}

	for _, c := range cases {
		if got := c.kind.IsInternal(); got != c.internal {
			t.Errorf("%s: IsInternal() = %v, want %v", c.name, got, c.internal)
		}
		if got := c.kind.String(); got != c.name {
			t.Errorf("String() = %q, want %q", got, c.name)
		}
	}
}

func TestLegRole_FeesAreDistinguishedFromPrincipal(t *testing.T) {
	cases := []struct {
		role  LegRole
		isFee bool
		name  string
	}{
		{RolePrincipal, false, "principal"},
		{RoleFee, true, "fee"},
		{RoleNetworkFee, true, "network fee"},
		{RoleAdjustment, false, "adjustment"},
	}

	for _, c := range cases {
		if got := c.role.IsFee(); got != c.isFee {
			t.Errorf("%s: IsFee() = %v, want %v", c.name, got, c.isFee)
		}
		if got := c.role.String(); got != c.name {
			t.Errorf("String() = %q, want %q", got, c.name)
		}
	}
}

func TestOperationValidate_MalformedInput_IsRejectedBeforeAnyCheckRuns(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Operation)
		wantHas string
	}{
		{"no id", func(o *Operation) { o.ID = "" }, "id is empty"},
		{"no legs", func(o *Operation) { o.Legs = nil }, "no legs"},
		{"leg without account", func(o *Operation) { o.Legs[1].Account.ID = "" }, "no account"},
		{"unclassified account", func(o *Operation) { o.Legs[1].Account.Kind = AccountUnknown }, "unclassified"},
		{"leg without currency", func(o *Operation) { o.Legs[1].Amount = Amount{} }, "no currency"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			operation := withdrawalBalanced()
			c.mutate(&operation)

			err := operation.Validate()
			if err == nil {
				t.Fatalf("expected %s to be rejected", c.name)
			}
			if !strings.Contains(err.Error(), c.wantHas) {
				t.Errorf("error %q does not mention %q", err, c.wantHas)
			}
		})
	}
}

func TestOperationValidate_WellFormed_IsAccepted(t *testing.T) {
	if err := withdrawalBalanced().Validate(); err != nil {
		t.Fatalf("well-formed operation rejected: %v", err)
	}
}

func TestOperationPrincipalCurrencies_NoPrincipalLegs_ReturnsNothing(t *testing.T) {
	operation := Operation{
		ID:   "op-fee-only",
		Legs: []Leg{leg("l1", feeAccount, "1", usdt, RoleFee)},
	}
	if got := operation.PrincipalCurrencies(); len(got) != 0 {
		t.Errorf("PrincipalCurrencies() = %v, want none", got)
	}
	if operation.HasPrincipalIn(usdt) {
		t.Error("a fee leg is not principal")
	}
}

// An exchange has two principal currencies and neither is more principal than
// the other. A singular answer here would be whichever sorted first, and every
// caller would then reason about an arbitrary half of the operation.
func TestOperationPrincipalCurrencies_Trade_ReturnsBoth(t *testing.T) {
	got := tradeBalanced().PrincipalCurrencies()
	if len(got) != 2 || got[0] != btc || got[1] != usdt {
		t.Errorf("PrincipalCurrencies() = %v, want [BTC USDT]", got)
	}
	for _, currency := range []Currency{btc, usdt} {
		if !tradeBalanced().HasPrincipalIn(currency) {
			t.Errorf("HasPrincipalIn(%s) = false", currency)
		}
	}
	if tradeBalanced().HasPrincipalIn(trx) {
		t.Error("HasPrincipalIn(TRX) = true on an operation with no TRX leg")
	}
}

func TestOperationFeesIn_ReturnsOnlyFeeLegsOfThatCurrency(t *testing.T) {
	operation := withdrawalFeeInTRXAccounted()

	if fees := operation.FeesIn(usdt); len(fees) != 0 {
		t.Errorf("expected no USDT fee legs, got %+v", fees)
	}
	fees := operation.FeesIn(trx)
	if len(fees) != 2 {
		t.Fatalf("expected both TRX fee legs, got %d", len(fees))
	}
	for _, fee := range fees {
		if !fee.Role.IsFee() {
			t.Errorf("leg %s is not a fee leg", fee.ID)
		}
	}
}
