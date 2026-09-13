package reconcile

// Fixtures for the fee and boundary detectors. Synthetic throughout: round
// amounts, invented account ids, nothing derived from any real ledger.

var coldAccount = Account{ID: "acct-treasury-cold", Kind: AccountSystem}

func amountPtr(a Amount) *Amount { return &a }

// payment is the same set of postings every time: the customer is debited 100,
// the operator keeps 1, and 99 leaves to the chain.
//
// The postings alone do not say which convention the payment followed. Only
// the declared amount does — declare 99 and the fee sat on top of it; declare
// 100 and it came out of it. That is the whole of mode 01, and it is why these
// fixtures differ in one field and nothing else.
func payment(id, declared string) Operation {
	operation := Operation{
		ID:       id,
		Kind:     OpWithdrawal,
		At:       fixtureTime,
		Ref:      "withdrawal:" + id,
		Declared: amountPtr(MustParseAmount(declared, usdt)),
		Legs: []Leg{
			leg("l1", userAccount, "-100", usdt, RolePrincipal),
			leg("l2", feeAccount, "1", usdt, RoleFee),
			leg("l3", chainAccount, "99", usdt, RolePrincipal),
		},
	}
	return operation
}

func paymentFeeOnTop(id string) Operation  { return payment(id, "99") }
func paymentFeeInside(id string) Operation { return payment(id, "100") }

// paymentDeclaringNeither declares a figure that matches no convention: more
// left the account than the payment said, and the fee does not explain it.
func paymentDeclaringNeither(id string) Operation { return payment(id, "95") }

// payoutWithEstimate quotes a fee before sending and charges another after.
func payoutWithEstimate(id, estimate, charged string) Operation {
	return Operation{
		ID:          id,
		Kind:        OpWithdrawal,
		At:          fixtureTime,
		Ref:         "withdrawal:" + id,
		FeeEstimate: amountPtr(MustParseAmount(estimate, trx)),
		Legs: []Leg{
			leg("l1", userAccount, "-100", usdt, RolePrincipal),
			leg("l2", chainAccount, "100", usdt, RolePrincipal),
			leg("l3", treasuryAccount, "-"+charged, trx, RoleNetworkFee),
			leg("l4", chainAccount, charged, trx, RoleNetworkFee),
		},
	}
}

// depositCrossing is a real deposit: value arrives from outside the ledger.
func depositCrossing() Operation {
	return Operation{
		ID:   "op-deposit",
		Kind: OpDeposit,
		At:   fixtureTime,
		Ref:  "chain:deposit:1",
		Legs: []Leg{
			leg("l1", chainAccount, "-100", usdt, RolePrincipal),
			leg("l2", userAccount, "100", usdt, RolePrincipal),
		},
	}
}

// sweepInternal moves value between two accounts the operator owns. It is a
// real transaction on the chain and no movement at all across the ledger's
// boundary: nothing entered and nothing left.
func sweepInternal() Operation {
	return Operation{
		ID:   "op-sweep",
		Kind: OpTransfer,
		At:   fixtureTime,
		Ref:  "chain:sweep:1",
		Legs: []Leg{
			leg("l1", treasuryAccount, "-5", usdt, RolePrincipal),
			leg("l2", coldAccount, "5", usdt, RolePrincipal),
		},
	}
}

// depositRecord is the chain's view of the deposit.
func depositRecord() ExternalRecord {
	return ExternalRecord{
		ID:     "tx-deposit",
		Ref:    "chain:deposit:1",
		Amount: MustParseAmount("100", usdt),
		At:     fixtureTime,
	}
}

// sweepRecord is the chain's view of the sweep, recorded as though value had
// arrived. This is the mistake mode 11 describes: the transaction is real and
// the entry is right, and counting it in an equation about deposits and
// withdrawals makes a correct ledger look broken.
func sweepRecord() ExternalRecord {
	return ExternalRecord{
		ID:     "tx-sweep",
		Ref:    "chain:sweep:1",
		Amount: MustParseAmount("5", usdt),
		At:     fixtureTime,
	}
}
