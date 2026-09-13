package reconcile

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Quote is a price for one currency in terms of another, at a moment.
//
// Rate is how many units of Counter one unit of Base buys. A quote carries its
// own timestamp because a quote without one cannot answer the only question
// that matters here, which is as of when.
type Quote struct {
	Base    Currency        `json:"base"`
	Counter Currency        `json:"counter"`
	Rate    decimal.Decimal `json:"rate"`
	At      time.Time       `json:"at"`
}

// pair identifies a rate series.
type pair struct{ base, counter Currency }

// quoteBook answers "what was the rate at this moment".
type quoteBook map[pair][]Quote

// newQuoteBook indexes quotes by pair, each series ordered in time.
func newQuoteBook(quotes []Quote) (quoteBook, error) {
	book := quoteBook{}
	for _, quote := range quotes {
		if quote.Base == "" || quote.Counter == "" {
			return nil, fmt.Errorf("rates: quote at %s has no pair", quote.At)
		}
		if quote.At.IsZero() {
			return nil, fmt.Errorf("rates: quote for %s/%s carries no timestamp, "+
				"so it cannot say as of when it applied", quote.Base, quote.Counter)
		}
		// A rate at or below zero is not a price. Used, it would invert or
		// erase a valuation and the spread would still look like an answer.
		if quote.Rate.Sign() <= 0 {
			return nil, fmt.Errorf("rates: quote for %s/%s at %s has a rate of %s, which is not a price",
				quote.Base, quote.Counter, quote.At.UTC().Format(time.RFC3339), quote.Rate)
		}
		key := pair{base: quote.Base, counter: quote.Counter}
		book[key] = append(book[key], quote)
	}
	for key := range book {
		series := book[key]
		sort.SliceStable(series, func(i, j int) bool { return series[i].At.Before(series[j].At) })
		book[key] = series
	}
	return book, nil
}

// inForce returns the last quote at or before a moment.
func (b quoteBook) inForce(key pair, moment time.Time) (Quote, bool) {
	series := b[key]
	found := sort.Search(len(series), func(i int) bool { return series[i].At.After(moment) })
	if found == 0 {
		return Quote{}, false
	}
	return series[found-1], true
}

// Rates reports mode 07: one transaction, three different figures.
//
// The operation is valued in the counter currency at the rate in force at each
// moment it passed through, and the spread between those valuations is
// reported when it exceeds the tolerance. The finding carries all three
// figures with the timestamps that produced them, because the useful answer is
// not that they differ but which moment each report used.
func Rates(operations []Operation, quotes []Quote, counter Currency, tolerance Tolerance) ([]Finding, error) {
	book, err := newQuoteBook(quotes)
	if err != nil {
		return nil, err
	}
	if counter == "" {
		return nil, fmt.Errorf("rates: no counter currency given to value operations in")
	}

	var findings []Finding
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		if operation.Timeline == nil {
			continue
		}
		if finding, ok := rateFinding(operation, book, counter, tolerance); ok {
			findings = append(findings, finding)
		}
	}
	return sortFindings(findings), nil
}

// valuation is one operation valued at one moment.
type valuation struct {
	moment string
	at     time.Time
	rate   decimal.Decimal
	value  Amount
}

// valuationBase picks the currency an operation should be valued from.
//
// It is the one principal currency that is not already the currency being
// valued in. An operation with two such currencies has no single answer —
// valuing an exchange "in" one of its own sides is a different question from
// the one this mode asks — so it is left alone rather than valued from
// whichever currency happened to sort first.
func valuationBase(operation Operation, counter Currency) (Currency, bool) {
	var found Currency
	for _, currency := range operation.PrincipalCurrencies() {
		if currency == counter {
			continue
		}
		if found != "" {
			return "", false
		}
		found = currency
	}
	return found, found != ""
}

// rateFinding values one operation at each of its moments and reports the
// spread if it is wider than the tolerance allows.
func rateFinding(operation Operation, book quoteBook, counter Currency,
	tolerance Tolerance) (Finding, bool) {
	base, ok := valuationBase(operation, counter)
	if !ok {
		return Finding{}, false
	}
	moved := netOn(operation, base, isUser).Abs()
	if moved.IsZero() {
		return Finding{}, false
	}

	valuations := valueAtEachMoment(operation, book, pair{base: base, counter: counter}, moved)
	if len(valuations) < 2 {
		return Finding{}, false
	}

	lowest, highest := valuations[0], valuations[0]
	for _, v := range valuations {
		if cmp, _ := v.value.Cmp(lowest.value); cmp < 0 {
			lowest = v
		}
		if cmp, _ := v.value.Cmp(highest.value); cmp > 0 {
			highest = v
		}
	}

	spread, err := highest.value.Sub(lowest.value)
	if err != nil {
		return Finding{}, false
	}
	if within, _ := spread.Cmp(tolerance.For(counter)); within <= 0 {
		return Finding{}, false
	}

	return rateSpreadFinding(operation, moved, counter, valuations, spread, lowest, highest), true
}

// valueAtEachMoment converts the moved amount at the rate in force at each
// recorded moment, skipping moments no quote covers.
func valueAtEachMoment(operation Operation, book quoteBook, key pair, moved Amount) []valuation {
	var valuations []valuation
	for _, moment := range operation.Timeline.moments() {
		quote, found := book.inForce(key, moment.At)
		if !found {
			continue
		}
		value, err := NewAmount(moved.Value().Mul(quote.Rate), key.counter)
		if err != nil {
			continue
		}
		valuations = append(valuations, valuation{
			moment: moment.Name, at: moment.At, rate: quote.Rate, value: value,
		})
	}
	return valuations
}

// rateSpreadFinding writes out every valuation so the reader can see which
// moment each report must have used.
func rateSpreadFinding(operation Operation, moved Amount, counter Currency,
	valuations []valuation, spread Amount, lowest, highest valuation) Finding {
	summary := fmt.Sprintf("%s is worth %s at the %s rate and %s at the %s rate: a spread of %s on one transaction",
		moved, lowest.value, lowest.moment, highest.value, highest.moment, spread)

	arithmetic := fmt.Sprintf("%s valued in %s:", moved, counter)
	for _, v := range valuations {
		arithmetic += fmt.Sprintf(" %s (%s) x %s = %s;",
			v.moment, v.at.UTC().Format(time.RFC3339), v.rate, v.value.Value())
	}
	arithmetic += fmt.Sprintf(" spread = %s", spread.Value())

	return newFinding(ModeRateAsOf, operation.ID, counter, summary, arithmetic, nil)
}
