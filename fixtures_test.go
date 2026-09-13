package reconcile

import "time"

// Fixtures are written by hand and are entirely synthetic: the amounts are
// round, the account ids are obviously made up, and nothing here is derived
// from any real ledger. That is a rule of this repository, not an accident of
// convenience.

const (
	usdt Currency = "USDT"
	trx  Currency = "TRX"
	btc  Currency = "BTC"
)

var fixtureTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// at builds an instant on 2 January 2026 in UTC, which is the day the
// time-sensitive fixtures are set on.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 1, day, hour, minute, 0, 0, time.UTC)
}

// berlin is a fixed offset rather than a zone loaded from the operating
// system's database, so the fixtures mean the same thing on every machine and
// in any year.
var berlin = time.FixedZone("Europe/Berlin", 1*60*60)

var (
	userAccount     = Account{ID: "acct-user-1", Kind: AccountUser}
	feeAccount      = Account{ID: "acct-fee", Kind: AccountFee}
	treasuryAccount = Account{ID: "acct-treasury", Kind: AccountSystem}
	chainAccount    = Account{ID: "chain-tron", Kind: AccountExternal}
)

// leg is a shorthand for building one posting from a decimal string.
func leg(id string, account Account, value string, currency Currency, role LegRole) Leg {
	return Leg{
		ID:      id,
		Account: account,
		Amount:  MustParseAmount(value, currency),
		Role:    role,
	}
}

// withdrawalBalanced is a well-formed payout: the customer is debited, the
// operator keeps a fee, and the rest leaves to the chain. Every currency sums
// to zero, so no detector should have anything to say about it.
func withdrawalBalanced() Operation {
	return Operation{
		ID:   "op-withdraw-ok",
		Kind: OpWithdrawal,
		At:   fixtureTime,
		Ref:  "withdrawal:ok:1",
		Legs: []Leg{
			leg("l1", userAccount, "-100", usdt, RolePrincipal),
			leg("l2", feeAccount, "1", usdt, RoleFee),
			leg("l3", chainAccount, "99", usdt, RolePrincipal),
		},
	}
}

// withdrawalFeeInTRX is mode 14. The payout settles in USDT and balances
// there, so a check denominated in USDT reports that the payout cost nothing.
// The network fee was actually paid in TRX and nothing accounts for it.
func withdrawalFeeInTRX() Operation {
	return Operation{
		ID:   "op-withdraw-fee-trx",
		Kind: OpWithdrawal,
		At:   fixtureTime,
		Ref:  "withdrawal:fee-trx:1",
		Legs: []Leg{
			leg("l1", userAccount, "-100", usdt, RolePrincipal),
			leg("l2", chainAccount, "100", usdt, RolePrincipal),
			leg("l3", treasuryAccount, "-0.5", trx, RoleNetworkFee),
		},
	}
}

// withdrawalFeeInTRXAccounted is the negative case for mode 14: the same
// payout with the TRX leg mirrored, so both currencies balance.
func withdrawalFeeInTRXAccounted() Operation {
	operation := withdrawalFeeInTRX()
	operation.ID = "op-withdraw-fee-trx-ok"
	operation.Legs = append(operation.Legs,
		leg("l4", chainAccount, "0.5", trx, RoleNetworkFee))
	return operation
}

// tradeBalanced exchanges USDT for BTC in one operation, balancing in both
// currencies.
func tradeBalanced() Operation {
	return Operation{
		ID:   "op-trade-ok",
		Kind: OpTrade,
		At:   fixtureTime,
		Legs: []Leg{
			leg("l1", userAccount, "-100", usdt, RolePrincipal),
			leg("l2", treasuryAccount, "100", usdt, RolePrincipal),
			leg("l3", userAccount, "0.0025", btc, RolePrincipal),
			leg("l4", treasuryAccount, "-0.0025", btc, RolePrincipal),
		},
	}
}

// tradeMissingOneLeg is the trade with the BTC counter-leg dropped. The USDT
// side still balances, which is exactly why a checker that summed both
// currencies into one figure would have to net a BTC quantity against a USDT
// quantity to notice anything at all.
func tradeMissingOneLeg() Operation {
	operation := tradeBalanced()
	operation.ID = "op-trade-missing-leg"
	operation.Legs = operation.Legs[:3]
	return operation
}
