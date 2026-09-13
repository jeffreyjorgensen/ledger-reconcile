package reconcile

import "github.com/shopspring/decimal"

// The helpers here answer the questions every mechanism asks of an operation:
// what did these accounts do, in this currency, and how much of it was fee.
// They all work in one currency at a time, because there is no useful answer
// that spans two.

// zeroAmount returns nothing, denominated in a currency.
//
// Nothing still has a currency. "No USDT" and "no BTC" are different facts,
// and a zero that forgot which one it is will happily be compared with either.
func zeroAmount(currency Currency) Amount {
	return Amount{value: decimal.Zero, currency: currency}
}

// amountOrZero keeps an arithmetic line honest when a figure was never set.
func amountOrZero(amount Amount, currency Currency) Amount {
	if amount.Currency() != currency {
		return zeroAmount(currency)
	}
	return amount.Abs()
}

// legsIn returns the legs of an operation denominated in one currency.
func legsIn(operation Operation, currency Currency) []Leg {
	var legs []Leg
	for _, leg := range operation.Legs {
		if leg.Amount.Currency() == currency {
			legs = append(legs, leg)
		}
	}
	return legs
}

// netOn sums, in one currency, the legs posted against accounts the predicate
// accepts. The result keeps its sign: positive is value that arrived at those
// accounts, negative is value that left them.
func netOn(operation Operation, currency Currency, match func(Account) bool) Amount {
	total := zeroAmount(currency)
	for _, leg := range operation.Legs {
		if leg.Amount.Currency() != currency || !match(leg.Account) {
			continue
		}
		summed, err := total.Add(leg.Amount)
		if err != nil {
			continue // unreachable: the currency was checked above.
		}
		total = summed
	}
	return total
}

// feeTotal returns how much was charged in fees in one currency, as a
// magnitude. Both the operator's fee and the network's count: the payer does
// not care which of the two took the money.
//
// A fee has two legs — the account it left and the account it reached — so
// adding the magnitude of every fee leg would charge it twice. The figure that
// matters is what was collected, which is the credited side. A fee posted only
// as a debit, with nothing on the other end, falls back to the debited side:
// that operation is broken, and conservation will say so, but reporting its
// fee as zero here would hide it a second time.
func feeTotal(operation Operation, currency Currency) Amount {
	credited, debited := zeroAmount(currency), zeroAmount(currency)
	for _, leg := range operation.FeesIn(currency) {
		if leg.Amount.Sign() >= 0 {
			credited, _ = credited.Add(leg.Amount)
			continue
		}
		debited, _ = debited.Add(leg.Amount.Abs())
	}
	if credited.IsZero() {
		return debited
	}
	return credited
}

// isExternal and isUser are the predicates netOn is asked for most often.
func isExternal(account Account) bool { return account.Kind == AccountExternal }
func isUser(account Account) bool     { return account.Kind == AccountUser }
