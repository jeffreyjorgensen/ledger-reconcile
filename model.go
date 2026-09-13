package reconcile

import (
	"fmt"
	"time"
)

// AccountKind says which side of the boundary an account sits on.
//
// The distinction matters more than it looks. Movements between two internal
// accounts never cross to the outside world and must not be expected to have
// an external counterpart — expecting one is mode 11, and it is the most
// common way a correct ledger is reported as broken.
type AccountKind int

const (
	// AccountUnknown is the zero value and is always an input defect. A
	// classification this library had to guess at would be a classification
	// its findings could not defend.
	AccountUnknown AccountKind = iota

	// AccountUser is a customer balance.
	AccountUser

	// AccountFee is where fees the operator charged are collected.
	AccountFee

	// AccountSystem is an operator-owned working account: treasury, float,
	// suspense.
	AccountSystem

	// AccountExternal is the counterparty outside the ledger — the chain, the
	// provider, the bank. It holds the mirror of what the ledger owes or is
	// owed, so its position is routinely negative and that is not an error.
	AccountExternal
)

// IsInternal reports whether the account is inside the ledger's boundary.
func (k AccountKind) IsInternal() bool {
	return k == AccountUser || k == AccountFee || k == AccountSystem
}

// String names the kind for findings.
func (k AccountKind) String() string {
	switch k {
	case AccountUser:
		return "user"
	case AccountFee:
		return "fee"
	case AccountSystem:
		return "system"
	case AccountExternal:
		return "external"
	default:
		return "unclassified"
	}
}

// Account identifies one account and which side of the boundary it is on.
type Account struct {
	ID   string      `json:"id"`
	Kind AccountKind `json:"kind"`
}

// LegRole says what part a leg plays in its operation.
type LegRole int

const (
	// RolePrincipal is the value the operation exists to move.
	RolePrincipal LegRole = iota

	// RoleFee is what the operator charged for moving it.
	RoleFee

	// RoleNetworkFee is what the network charged. It is separate from RoleFee
	// because it is frequently paid in a different asset, which is mode 14.
	RoleNetworkFee

	// RoleAdjustment is a correction posted against the operation.
	RoleAdjustment
)

// String names the role for findings.
func (r LegRole) String() string {
	switch r {
	case RolePrincipal:
		return "principal"
	case RoleFee:
		return "fee"
	case RoleNetworkFee:
		return "network fee"
	case RoleAdjustment:
		return "adjustment"
	default:
		return "unknown"
	}
}

// IsFee reports whether the leg is a charge rather than the value being moved.
func (r LegRole) IsFee() bool { return r == RoleFee || r == RoleNetworkFee }

// BalancePart says which part of an account's balance a leg moves.
//
// An account holds two figures, not one: what the owner may spend and what is
// reserved against something already promised. A hold is two legs on the same
// account in the same currency — available down, held up — so it sums to zero
// and conservation is undisturbed, while the spendable figure falls.
//
// Mode 04 is what happens when a system keeps only the first figure: two
// operations each check the same balance, each sees enough, and the second one
// spends money the first has already committed.
type BalancePart int

const (
	// BalanceAvailable is what the owner may spend. It is the default because
	// most legs move spendable value.
	BalanceAvailable BalancePart = iota

	// BalanceHeld is what is reserved and may not be spent again.
	BalanceHeld
)

// String names the part for findings.
func (p BalancePart) String() string {
	if p == BalanceHeld {
		return "held"
	}
	return "available"
}

// Leg is one signed posting against one part of one account's balance.
//
// The companion article calls this a posting, which is the accounting word
// for it. The type is named Leg because the code speaks of an operation's
// legs far more often than of a single one, and "the legs sum to zero" is
// the sentence the whole package is built on.
//
// Positive is value arriving at the account, negative is value leaving it. An
// operation whose legs do not sum to zero in every currency has created or
// destroyed value without naming a source, which is what Conserve looks for.
type Leg struct {
	ID      string      `json:"id"`
	Account Account     `json:"account"`
	Amount  Amount      `json:"amount"`
	Role    LegRole     `json:"role"`
	Part    BalancePart `json:"part,omitempty"`
}

// OperationKind is what the operation was for. It is carried through to
// findings so a reader can tell a deposit's imbalance from a trade's.
type OperationKind string

// The kinds this library has met. The set is open: an unrecognised kind is
// carried through to findings unchanged rather than rejected, because what an
// operation was for is the caller's vocabulary, not this package's.
const (
	OpDeposit    OperationKind = "deposit"
	OpWithdrawal OperationKind = "withdrawal"
	OpTransfer   OperationKind = "transfer"
	OpTrade      OperationKind = "trade"
	OpAdjustment OperationKind = "adjustment"
)

// Operation is one indivisible movement of value, expressed as its legs.
//
// An exchange of one asset for another is one operation with legs in two
// currencies, not two operations. Splitting it would let the two halves settle
// at different times and leave a window in which value exists on one side and
// not the other.
type Operation struct {
	ID   string        `json:"id"`
	Kind OperationKind `json:"kind"`
	At   time.Time     `json:"at"`

	// Ref is the caller's idempotency key for the operation, of whatever shape
	// the caller uses. It is what mode 02 matches on.
	Ref string `json:"ref,omitempty"`

	Legs []Leg `json:"legs"`

	// Declared is the figure the payment instruction named, when the caller has
	// it. Mode 01 is the question of whether the fee sat on top of this figure
	// or inside it, and without the declared figure there is nothing to compare
	// the postings against.
	Declared *Amount `json:"declared,omitempty"`

	// BatchRef names the external transaction that carried this operation
	// together with others. One chain transaction routinely settles many
	// ledger operations — a batched payout, a sweep — and comparing either
	// against the other one at a time is mode 10.
	BatchRef string `json:"batch_ref,omitempty"`

	// Reverses is the id of the operation this one undoes, when it undoes one.
	// Without it a refund is indistinguishable from a fresh payment in the
	// opposite direction, which is mode 06.
	Reverses string `json:"reverses,omitempty"`

	// Timeline is the moments the operation passed through, when the caller
	// records them. Mode 07 needs them: a figure converted at the moment the
	// customer was quoted differs from the same figure converted when it
	// executed and again when it settled, and a system that does not say which
	// moment it used will report all three at different times.
	Timeline *Timeline `json:"timeline,omitempty"`

	// FeeEstimate is what the fee was quoted as before the operation was sent.
	// It is not a leg: a quote moves no value, and treating it as one would
	// unbalance every operation that carries it. Mode 12 compares it with what
	// the fee legs actually came to.
	FeeEstimate *Amount `json:"fee_estimate,omitempty"`
}

// Timeline is the moments one operation passed through.
//
// They are separate fields rather than one timestamp because they are separate
// facts. Between being quoted and settling, an operation can cross a move in
// the rate large enough that the customer's statement, the operator's revenue
// report and the tax figure are three different numbers, each defensible, for
// one transaction.
type Timeline struct {
	Ordered  time.Time `json:"ordered,omitempty"`
	Executed time.Time `json:"executed,omitempty"`
	Settled  time.Time `json:"settled,omitempty"`
}

// moments returns the timeline's named moments, in the order they occur, and
// skips the ones the caller did not record.
func (t Timeline) moments() []struct {
	Name string
	At   time.Time
} {
	all := []struct {
		Name string
		At   time.Time
	}{
		{"ordered", t.Ordered},
		{"executed", t.Executed},
		{"settled", t.Settled},
	}

	var recorded []struct {
		Name string
		At   time.Time
	}
	for _, moment := range all {
		if !moment.At.IsZero() {
			recorded = append(recorded, moment)
		}
	}
	return recorded
}

// Balance is what one account holds in one currency at one moment.
//
// Three figures, not one. The number shown to the customer is Available; what
// they can actually take out is smaller by whatever is frozen, whatever must
// stay behind, and whatever is too small to move. Mode 13 is the distance
// between the two.
type Balance struct {
	Account Account `json:"account"`

	Available Amount `json:"available"`
	Held      Amount `json:"held"`

	// Frozen is value blocked by something other than a pending operation: a
	// legal hold, a compliance block, a dispute.
	Frozen Amount `json:"frozen,omitempty"`
}

// validate rejects a balance this file cannot reason about.
func (b Balance) validate() error {
	if b.Account.ID == "" {
		return fmt.Errorf("balance: no account")
	}
	if b.Account.Kind == AccountUnknown {
		return fmt.Errorf("balance on %s: account is unclassified", b.Account.ID)
	}
	if b.Available.Currency() == "" {
		return fmt.Errorf("balance on %s: available amount has no currency", b.Account.ID)
	}
	return nil
}

// ExternalRecord is one record from the other side of the boundary: a chain
// transaction, a provider statement line, a bank entry.
//
// Amount is signed from the ledger's point of view — positive is value that
// arrived, negative is value that left. Ref is whatever key ties the record to
// the operation that was supposed to produce it.
type ExternalRecord struct {
	ID     string    `json:"id"`
	Ref    string    `json:"ref"`
	Amount Amount    `json:"amount"`
	At     time.Time `json:"at"`

	// Counterparty is who the value moved to or from, as the outside world
	// names them: an address, an account number, a provider-side id. It is
	// what a duplicate can be recognised by when no request id was carried.
	Counterparty string `json:"counterparty,omitempty"`

	// Chain names the ledger the record came from, and is the key the finality
	// rules are looked up under. An empty chain means the caller is not making
	// a claim about reversibility, and mode 09 skips the record.
	Chain string `json:"chain,omitempty"`

	// Confirmations is how deep the transaction was buried when the record was
	// taken. It is a proxy for finality on chains that have no better answer,
	// and it is not finality itself — which is the whole of mode 09.
	Confirmations int `json:"confirmations,omitempty"`

	// Final is set when the chain itself has declared the transaction
	// irreversible. On chains that publish finality this is the only answer
	// that means anything, and depth means nothing.
	Final bool `json:"final,omitempty"`

	// Reorged is set when the transaction that produced this record was later
	// removed from the chain.
	Reorged bool `json:"reorged,omitempty"`
}

// Validate rejects an operation this library cannot reason about.
//
// It is called before every check rather than after, because a detector that
// runs on malformed input reports the malformation as a discrepancy and sends
// the reader looking in the wrong place.
func (o Operation) Validate() error {
	if o.ID == "" {
		return fmt.Errorf("operation: id is empty")
	}
	if len(o.Legs) == 0 {
		return fmt.Errorf("operation %s: no legs", o.ID)
	}
	for i, leg := range o.Legs {
		if leg.Account.ID == "" {
			return fmt.Errorf("operation %s: leg %d has no account", o.ID, i)
		}
		if leg.Account.Kind == AccountUnknown {
			return fmt.Errorf("operation %s: account %s is unclassified", o.ID, leg.Account.ID)
		}
		if leg.Amount.Currency() == "" {
			return fmt.Errorf("operation %s: leg %d has no currency", o.ID, i)
		}
	}
	return nil
}

// Sum accumulates every leg of the operation, keeping currencies apart.
func (o Operation) Sum() *Sum {
	total := NewSum()
	for _, leg := range o.Legs {
		total.Add(leg.Amount)
	}
	return total
}

// FeesIn returns the fee legs denominated in one currency.
func (o Operation) FeesIn(currency Currency) []Leg {
	var fees []Leg
	for _, leg := range o.Legs {
		if leg.Role.IsFee() && leg.Amount.Currency() == currency {
			fees = append(fees, leg)
		}
	}
	return fees
}

// PrincipalCurrencies returns every currency the operation's principal legs
// are denominated in, in a stable order.
//
// There is deliberately no singular form. An exchange has two principal
// currencies and neither is more principal than the other; a function that had
// to return one would return whichever sorted first, and every caller would
// then be reasoning about an arbitrary half of the operation.
func (o Operation) PrincipalCurrencies() []Currency {
	principal := NewSum()
	for _, leg := range o.Legs {
		if leg.Role == RolePrincipal {
			principal.Add(leg.Amount)
		}
	}
	return principal.Currencies()
}

// HasPrincipalIn reports whether any principal leg is denominated in a
// currency. A currency that carries only fees is what mode 14 is about.
func (o Operation) HasPrincipalIn(currency Currency) bool {
	for _, leg := range o.Legs {
		if leg.Role == RolePrincipal && leg.Amount.Currency() == currency {
			return true
		}
	}
	return false
}
