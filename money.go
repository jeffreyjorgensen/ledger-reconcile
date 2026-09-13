package reconcile

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// Currency is the unit an amount is denominated in: "USD", "BTC", "USDT".
//
// It is compared exactly, and two spellings of the same asset are two
// currencies. That is deliberate. Folding them together quietly is how a fee
// paid in one asset vanishes into a payout denominated in another, which is
// mode 14 and the reason this package never adds across currencies.
type Currency string

func (c Currency) String() string { return string(c) }

// Amount is a signed quantity in exactly one currency.
//
// The sign carries direction, not size: positive is value arriving at an
// account, negative is value leaving it. The legs of a well-formed operation
// therefore sum to zero, and that identity is the whole of Conserve.
//
// There is no constructor that takes a float, and there will not be one. A
// binary float cannot represent 0.1, and a library about money that loses a
// cent inside its own arithmetic has no argument left to make.
type Amount struct {
	value    decimal.Decimal
	currency Currency
}

// NewAmount builds an amount from an already-parsed decimal.
func NewAmount(value decimal.Decimal, currency Currency) (Amount, error) {
	if currency == "" {
		return Amount{}, fmt.Errorf("amount: currency is empty")
	}
	return Amount{value: value, currency: currency}, nil
}

// ParseAmount reads a decimal string such as "-12.50".
//
// It returns an error rather than a zero when the text will not parse. The two
// are not the same thing and the difference is expensive: a zero minimum reads
// as "no minimum", a zero fee reads as "free", and both are wrong answers to
// "this field was unreadable".
func ParseAmount(text string, currency Currency) (Amount, error) {
	if text == "" {
		return Amount{}, fmt.Errorf("amount: empty value for %s", currency)
	}
	value, err := decimal.NewFromString(text)
	if err != nil {
		return Amount{}, fmt.Errorf("amount: parse %q as %s: %w", text, currency, err)
	}
	return NewAmount(value, currency)
}

// MustParseAmount is ParseAmount for fixtures and package-level values, where
// an unparseable literal is a defect in the caller rather than bad input.
func MustParseAmount(text string, currency Currency) Amount {
	amount, err := ParseAmount(text, currency)
	if err != nil {
		panic(err)
	}
	return amount
}

// Value returns the signed quantity. Callers that need to compare or print it
// should prefer the methods on Amount, which will not let them lose the
// currency along the way.
func (a Amount) Value() decimal.Decimal { return a.value }

// Currency returns the unit the quantity is denominated in.
func (a Amount) Currency() Currency { return a.currency }

// IsZero reports whether the quantity is zero, regardless of currency.
func (a Amount) IsZero() bool { return a.value.IsZero() }

// Sign returns -1, 0 or +1.
func (a Amount) Sign() int { return a.value.Sign() }

// Neg returns the amount with its direction reversed.
func (a Amount) Neg() Amount {
	return Amount{value: a.value.Neg(), currency: a.currency}
}

// Abs returns the amount without its direction.
func (a Amount) Abs() Amount {
	return Amount{value: a.value.Abs(), currency: a.currency}
}

// Add returns a+b, or an error if the two are in different currencies.
//
// Refusing the mixed case is the point. A checker that let the two through
// would net a fee in one asset against a payout in another and report that
// everything balances, which is precisely the failure this package exists to
// catch.
func (a Amount) Add(b Amount) (Amount, error) {
	if a.currency != b.currency {
		return Amount{}, fmt.Errorf("amount: cannot add %s to %s", b.currency, a.currency)
	}
	return Amount{value: a.value.Add(b.value), currency: a.currency}, nil
}

// Sub returns a-b, or an error if the two are in different currencies.
func (a Amount) Sub(b Amount) (Amount, error) {
	return a.Add(b.Neg())
}

// Cmp compares two amounts of the same currency and returns -1, 0 or +1.
func (a Amount) Cmp(b Amount) (int, error) {
	if a.currency != b.currency {
		return 0, fmt.Errorf("amount: cannot compare %s with %s", a.currency, b.currency)
	}
	return a.value.Cmp(b.value), nil
}

// Equal reports whether two amounts are the same quantity in the same
// currency. Amounts in different currencies are never equal, including when
// both are zero: "nothing in USD" and "nothing in BTC" are different facts.
func (a Amount) Equal(b Amount) bool {
	return a.currency == b.currency && a.value.Equal(b.value)
}

// String renders the amount as "12.50 USD".
func (a Amount) String() string {
	if a.currency == "" {
		return a.value.String()
	}
	return a.value.String() + " " + string(a.currency)
}

// amountJSON is the wire form. The quantity travels as a string so that a
// JSON parser never gets the chance to round it through a float on the way in.
type amountJSON struct {
	Value    string   `json:"value"`
	Currency Currency `json:"currency"`
}

// MarshalJSON writes the amount with its quantity as a string.
func (a Amount) MarshalJSON() ([]byte, error) {
	return json.Marshal(amountJSON{Value: a.value.String(), Currency: a.currency})
}

// UnmarshalJSON reads an amount whose quantity is a string. A JSON number is
// rejected rather than accepted, because accepting it would mean the value had
// already passed through a float before this code ever saw it.
func (a *Amount) UnmarshalJSON(data []byte) error {
	var wire amountJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("amount: decode: %w", err)
	}
	parsed, err := ParseAmount(wire.Value, wire.Currency)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// Sum accumulates amounts while keeping every currency apart.
//
// This is the shape the domain actually has. A single running total forces a
// choice between refusing mixed input and silently converting it, and both are
// wrong: an operation with a fee in another asset is legitimate, and it must
// balance in each asset separately rather than on average.
type Sum struct {
	byCurrency map[Currency]decimal.Decimal
}

// NewSum returns an empty sum.
func NewSum() *Sum {
	return &Sum{byCurrency: make(map[Currency]decimal.Decimal)}
}

// Add accumulates one amount into the sum for its own currency.
func (s *Sum) Add(a Amount) {
	if s.byCurrency == nil {
		s.byCurrency = make(map[Currency]decimal.Decimal)
	}
	s.byCurrency[a.currency] = s.byCurrency[a.currency].Add(a.value)
}

// In returns the running total for one currency, zero if nothing was added.
func (s *Sum) In(currency Currency) Amount {
	return Amount{value: s.byCurrency[currency], currency: currency}
}

// Currencies returns every currency the sum has seen, in a stable order.
//
// Sorting is not cosmetic. Findings are compared in tests and read by people,
// and a set of findings whose order came from Go's map iteration would differ
// between runs on identical input — which would make the library's own claim
// of determinism false.
func (s *Sum) Currencies() []Currency {
	currencies := make([]Currency, 0, len(s.byCurrency))
	for currency := range s.byCurrency {
		currencies = append(currencies, currency)
	}
	sort.Slice(currencies, func(i, j int) bool { return currencies[i] < currencies[j] })
	return currencies
}

// NonZero returns the currencies whose running total is not zero, in a stable
// order. An operation that accounts for itself returns nothing here.
func (s *Sum) NonZero() []Amount {
	var residuals []Amount
	for _, currency := range s.Currencies() {
		if amount := s.In(currency); !amount.IsZero() {
			residuals = append(residuals, amount)
		}
	}
	return residuals
}

// IsBalanced reports whether every currency in the sum totals zero.
func (s *Sum) IsBalanced() bool { return len(s.NonZero()) == 0 }
