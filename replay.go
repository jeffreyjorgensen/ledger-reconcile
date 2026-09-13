package reconcile

import (
	"fmt"
	"sort"
)

// Replay reports mode 04: the balance went negative despite the check.
//
// This is not a comparison between two sources. It is a reconstruction. The
// operations are walked in time order while the available and held parts of
// every internal balance are maintained, and the first operation that drives a
// spendable balance below zero is reported together with what was reserved at
// that moment — which, in the case this mode describes, is nothing.
//
// The failure it catches is a race that leaves no trace in the data: two
// operations check the same balance, both see enough, and the second spends
// money the first has already committed. Afterwards every entry is individually
// correct and the balance is impossible. Only the order shows it.
//
// Opening carries the state every internal balance was in before the first
// operation, and every internal account and currency the operations touch must
// appear in it. This is not bureaucracy. Starting from an assumed zero would
// report every account funded before the window as overdrawn — a sweep out of
// a treasury that was full yesterday becomes a finding — and "went negative"
// and "started positive and we were not told" are not distinguishable after
// the fact. An account that really begins empty is passed as an explicit zero.
//
// Each account and currency is reported once, at its first breach. Everything
// after the first is a consequence of it and would bury the cause.
func Replay(operations []Operation, opening []Balance) ([]Finding, error) {
	ordered, err := inTimeOrder(operations)
	if err != nil {
		return nil, err
	}

	balances, err := openingBalances(opening, ordered)
	if err != nil {
		return nil, err
	}
	reported := map[balanceKey]bool{}
	var findings []Finding

	for _, operation := range ordered {
		balances.apply(operation)
		findings = append(findings, balances.breaches(operation, reported)...)
	}
	return sortFindings(findings), nil
}

// inTimeOrder sorts a copy of the operations by time, breaking ties by id.
//
// The tie-break is not cosmetic. Two operations carrying the same timestamp
// are common — a batch written in one transaction — and without a second key
// the order would come from however the caller happened to pass them, which
// would make the finding depend on the input's order rather than its content.
func inTimeOrder(operations []Operation) ([]Operation, error) {
	ordered := make([]Operation, len(operations))
	copy(ordered, operations)

	for _, operation := range ordered {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		// This check is entirely about order, so an operation that does not say
		// when it happened cannot take part in it. Sorted with a zero time it
		// would silently land before everything else and the answer would be an
		// artefact of the gap in the data.
		if operation.At.IsZero() {
			return nil, fmt.Errorf("replay: operation %s carries no timestamp, and this check "+
				"is about the order things happened in", operation.ID)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].At.Equal(ordered[j].At) {
			return ordered[i].At.Before(ordered[j].At)
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered, nil
}

// balanceKey identifies one running figure: one account, one currency, one
// part of the balance.
type balanceKey struct {
	account  string
	currency Currency
	part     BalancePart
}

// balances is the reconstructed state as the replay walks forward.
type balances struct {
	byKey map[balanceKey]Amount
	kinds map[string]AccountKind
	// touched keeps the accounts each operation moved, in a stable order, so
	// that the breach check does not iterate a map.
	touched []balanceKey
}

func newBalances() *balances {
	return &balances{byKey: map[balanceKey]Amount{}, kinds: map[string]AccountKind{}}
}

// openingBalances seeds the reconstruction and refuses to start without a
// stated position for every internal balance the operations touch.
func openingBalances(opening []Balance, operations []Operation) (*balances, error) {
	balances := newBalances()
	for _, balance := range opening {
		if err := balance.validate(); err != nil {
			return nil, fmt.Errorf("replay: opening %w", err)
		}
		currency := balance.Available.Currency()
		account := balance.Account.ID
		balances.byKey[balanceKey{account: account, currency: currency, part: BalanceAvailable}] =
			balance.Available
		balances.byKey[balanceKey{account: account, currency: currency, part: BalanceHeld}] =
			amountOrZero(balance.Held, currency)
		balances.kinds[account] = balance.Account.Kind
	}

	for _, operation := range operations {
		for _, leg := range operation.Legs {
			if !leg.Account.Kind.IsInternal() {
				continue
			}
			key := balanceKey{
				account:  leg.Account.ID,
				currency: leg.Amount.Currency(),
				part:     BalanceAvailable,
			}
			if _, stated := balances.byKey[key]; !stated {
				return nil, fmt.Errorf(
					"replay: no opening balance given for %s in %s, and a replay from an assumed "+
						"zero would report an account funded before this window as overdrawn; "+
						"pass an explicit zero if it really began empty",
					leg.Account.ID, leg.Amount.Currency())
			}
		}
	}
	return balances, nil
}

// apply posts every leg of one operation and records what it touched.
func (b *balances) apply(operation Operation) {
	b.touched = b.touched[:0]
	for _, leg := range operation.Legs {
		key := balanceKey{
			account:  leg.Account.ID,
			currency: leg.Amount.Currency(),
			part:     leg.Part,
		}
		current, seen := b.byKey[key]
		if !seen {
			current = zeroAmount(key.currency)
		}
		updated, err := current.Add(leg.Amount)
		if err != nil {
			continue // unreachable: the key carries the leg's own currency.
		}
		b.byKey[key] = updated
		b.kinds[leg.Account.ID] = leg.Account.Kind
		b.touched = append(b.touched, key)
	}
}

// breaches reports every spendable balance the last operation drove below zero
// and has not been reported before.
func (b *balances) breaches(operation Operation, reported map[balanceKey]bool) []Finding {
	var findings []Finding
	for _, key := range b.touched {
		if key.part != BalanceAvailable || !b.kinds[key.account].IsInternal() {
			continue
		}
		if b.byKey[key].Sign() >= 0 || reported[key] {
			continue
		}
		reported[key] = true
		findings = append(findings, b.breachFinding(operation, key))
	}
	return findings
}

// breachFinding describes one balance that went negative.
func (b *balances) breachFinding(operation Operation, key balanceKey) Finding {
	available := b.byKey[key]
	held := b.byKey[balanceKey{account: key.account, currency: key.currency, part: BalanceHeld}]
	if held.Currency() == "" {
		held = zeroAmount(key.currency)
	}

	summary := fmt.Sprintf(
		"%s spendable balance fell to %s: the check that allowed this saw a figure "+
			"nothing was reserved against",
		key.account, available)
	arithmetic := fmt.Sprintf(
		"after %s: available = %s, held = %s. Covering this would have needed a hold "+
			"of %s placed before the earlier operation settled",
		operation.ID, available.Value(), held.Value(), available.Abs().Value())

	return newFinding(ModeMissingHold, operation.ID, key.currency, summary, arithmetic,
		evidenceFrom(legsOn(operation, key.account, key.currency)))
}

// legsOn returns the legs of an operation posted against one account in one
// currency.
func legsOn(operation Operation, account string, currency Currency) []Leg {
	var legs []Leg
	for _, leg := range operation.Legs {
		if leg.Account.ID == account && leg.Amount.Currency() == currency {
			legs = append(legs, leg)
		}
	}
	return legs
}
