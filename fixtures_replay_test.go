package reconcile

// Fixtures for the replay detectors. Synthetic throughout.

// funded credits the customer with 100 and is the starting point for both
// payment sequences below.
func funded() Operation {
	return Operation{
		ID: "op-1-fund", Kind: OpDeposit, At: at(2, 9, 0), Ref: "chain:deposit:seed",
		Legs: []Leg{
			leg("l1", chainAccount, "-100", usdt, RolePrincipal),
			leg("l2", userAccount, "100", usdt, RolePrincipal),
		},
	}
}

// payment sends value straight out of the spendable balance, reserving
// nothing. Two of these in flight at once is mode 04: each checks the balance,
// each sees 100, and between the check and the settlement nothing marks the
// money as already promised.
func paymentWithoutHold(id, amount string, hour int) Operation {
	return Operation{
		ID: id, Kind: OpWithdrawal, At: at(2, hour, 0), Ref: "withdrawal:" + id,
		Legs: []Leg{
			leg("l1", userAccount, "-"+amount, usdt, RolePrincipal),
			leg("l2", chainAccount, amount, usdt, RolePrincipal),
		},
	}
}

// holdFor reserves value: it leaves the spendable part and enters the held
// part of the same account, so the operation still sums to zero while the
// figure a later check reads has already fallen.
func holdFor(id, amount string, hour int) Operation {
	return Operation{
		ID: id, Kind: OpTransfer, At: at(2, hour, 0), Ref: "hold:" + id,
		Legs: []Leg{
			{ID: "l1", Account: userAccount, Amount: MustParseAmount("-"+amount, usdt),
				Role: RolePrincipal, Part: BalanceAvailable},
			{ID: "l2", Account: userAccount, Amount: MustParseAmount(amount, usdt),
				Role: RolePrincipal, Part: BalanceHeld},
		},
	}
}

// settleHeld sends out value that was reserved beforehand.
func settleHeld(id, amount string, hour int) Operation {
	return Operation{
		ID: id, Kind: OpWithdrawal, At: at(2, hour, 0), Ref: "withdrawal:" + id,
		Legs: []Leg{
			{ID: "l1", Account: userAccount, Amount: MustParseAmount("-"+amount, usdt),
				Role: RolePrincipal, Part: BalanceHeld},
			{ID: "l2", Account: chainAccount, Amount: MustParseAmount(amount, usdt),
				Role: RolePrincipal, Part: BalanceAvailable},
		},
	}
}

// nearMidnight is an operation timed so that the reporting zone and UTC put it
// on different dates: 00:30 in Berlin on the 2nd is 23:30 UTC on the 1st.
func nearMidnight(id string) Operation {
	return Operation{
		ID: id, Kind: OpDeposit, At: at(1, 23, 30), Ref: "chain:deposit:" + id,
		Legs: []Leg{
			leg("l1", chainAccount, "-40", usdt, RolePrincipal),
			leg("l2", userAccount, "40", usdt, RolePrincipal),
		},
	}
}

// middayOperation sits far from any boundary and belongs to the same date in
// both interpretations.
func middayOperation(id string) Operation {
	return Operation{
		ID: id, Kind: OpDeposit, At: at(2, 11, 0), Ref: "chain:deposit:" + id,
		Legs: []Leg{
			leg("l1", chainAccount, "-25", usdt, RolePrincipal),
			leg("l2", userAccount, "25", usdt, RolePrincipal),
		},
	}
}
