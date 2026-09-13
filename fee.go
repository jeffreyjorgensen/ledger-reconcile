package reconcile

import "fmt"

// Tolerance is how far a figure may drift from its estimate, per currency,
// before it is worth reporting.
//
// A currency with no entry has a tolerance of zero, so any difference at all
// is reported. That is the safe direction: an unconfigured tolerance should
// make the library noisier, never quieter.
type Tolerance map[Currency]Amount

// For returns the tolerance for one currency, zero when none was configured.
func (t Tolerance) For(currency Currency) Amount {
	if amount, ok := t[currency]; ok {
		return amount.Abs()
	}
	return zeroAmount(currency)
}

// convention is which side of a payment the fee was taken from.
type convention int

const (
	conventionUnknown convention = iota

	// conventionOnTop: the recipient is sent the declared amount and the
	// sender is debited the declared amount plus the fee.
	conventionOnTop

	// conventionInside: the sender is debited the declared amount and the
	// recipient receives it less the fee.
	conventionInside
)

func (c convention) String() string {
	switch c {
	case conventionOnTop:
		return "fee on top of the declared amount"
	case conventionInside:
		return "fee taken out of the declared amount"
	default:
		return "neither convention"
	}
}

// FeeConventions reports mode 01: more left the account than the payment said.
//
// The failure is rarely a wrong figure. It is two conventions coexisting in
// one system — some payments charging the fee on top of the declared amount
// and some taking it out of the declared amount — so every individual payment
// looks defensible and the totals still do not agree. This reports payments
// that match neither convention, and, where a set uses both, the ones in the
// minority.
//
// Operations without a declared amount are skipped: there is nothing to
// compare the postings against, and guessing would be worse than silence.
func FeeConventions(operations []Operation) ([]Finding, error) {
	var findings []Finding
	seen := map[convention]int{}
	byConvention := map[convention][]Operation{}

	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		if operation.Declared == nil {
			continue
		}

		matched, applicable := conventionOf(operation, *operation.Declared)
		switch {
		case !applicable:
			// No fee was charged, so the question this mode asks does not
			// arise. Putting the payment in a camp it never chose would then
			// make it the minority in some later set and produce a finding out
			// of nothing.
		case matched == conventionUnknown:
			findings = append(findings, neitherConventionFinding(operation, *operation.Declared))
		default:
			seen[matched]++
			byConvention[matched] = append(byConvention[matched], operation)
		}
	}

	findings = append(findings, mixedConventionFindings(seen, byConvention)...)
	return sortFindings(findings), nil
}

// conventionOf decides which convention one payment follows.
//
// The second return says whether the question applies at all. With no fee
// charged the two conventions are the same statement, and the payment belongs
// in neither camp — which is not the same as matching neither, and must not be
// reported as though it were.
func conventionOf(operation Operation, declared Amount) (convention, bool) {
	currency := declared.Currency()
	fee := feeTotal(operation, currency)
	if fee.IsZero() {
		return conventionUnknown, false
	}

	toOutside := netOn(operation, currency, isExternal)
	debited := netOn(operation, currency, isUser).Neg()

	declaredPlusFee, _ := declared.Add(fee)
	declaredLessFee, _ := declared.Sub(fee)

	switch {
	case toOutside.Equal(declared) && debited.Equal(declaredPlusFee):
		return conventionOnTop, true
	case debited.Equal(declared) && toOutside.Equal(declaredLessFee):
		return conventionInside, true
	default:
		return conventionUnknown, true
	}
}

// neitherConventionFinding reports a payment whose postings match neither way
// of charging the fee — the case where more left the account than the payment
// said and nothing in the data explains the difference.
func neitherConventionFinding(operation Operation, declared Amount) Finding {
	currency := declared.Currency()
	fee := feeTotal(operation, currency)
	toOutside := netOn(operation, currency, isExternal)
	debited := netOn(operation, currency, isUser).Neg()

	summary := fmt.Sprintf(
		"payment declared %s but debited %s and sent %s: the fee of %s sits on neither side",
		declared, debited, toOutside, fee)

	arithmetic := fmt.Sprintf(
		"declared = %s; debited = %s; sent = %s; fee = %s. "+
			"fee on top would need sent == declared and debited == declared + fee; "+
			"fee inside would need debited == declared and sent == declared - fee",
		declared.Value(), debited.Value(), toOutside.Value(), fee.Value())

	return newFinding(ModeFeeConvention, operation.ID, currency,
		summary, arithmetic, evidenceFrom(legsIn(operation, currency)))
}

// mixedConventionFindings reports payments when a set uses both conventions.
//
// One convention applied consistently is a decision; two applied side by side
// is the mode. Where one camp is smaller it is named, because that is the one
// to look at. Where the two are the same size there is no minority, and
// picking a side would be a coin toss reported as a finding — so every payment
// is named and the summary says the set is split.
func mixedConventionFindings(seen map[convention]int, byConvention map[convention][]Operation) []Finding {
	onTop, inside := seen[conventionOnTop], seen[conventionInside]
	if onTop == 0 || inside == 0 {
		return nil
	}

	camps := []convention{conventionOnTop, conventionInside}
	even := onTop == inside
	if !even {
		minority := conventionOnTop
		if onTop > inside {
			minority = conventionInside
		}
		camps = []convention{minority}
	}

	var findings []Finding
	for _, camp := range camps {
		for _, operation := range byConvention[camp] {
			findings = append(findings, conventionFinding(operation, camp, seen, even))
		}
	}
	return findings
}

// conventionFinding describes one payment caught in a set that uses both
// conventions.
func conventionFinding(operation Operation, camp convention,
	seen map[convention]int, even bool) Finding {
	currency := operation.Declared.Currency()

	other := conventionInside
	if camp == conventionInside {
		other = conventionOnTop
	}

	var summary string
	if even {
		summary = fmt.Sprintf(
			"the set is split evenly — %d payment(s) charge the %s and %d the %s — so this one "+
				"cannot be called the error without a decision about which convention to keep",
			seen[camp], camp, seen[other], other)
	} else {
		summary = fmt.Sprintf("payment charges the %s while %d of its neighbours charge the %s",
			camp, seen[other], other)
	}

	arithmetic := fmt.Sprintf("%d payment(s) %s, %d payment(s) %s; totals cannot agree while both are in use",
		seen[camp], camp, seen[other], other)

	return newFinding(ModeFeeConvention, operation.ID, currency,
		summary, arithmetic, evidenceFrom(legsIn(operation, currency)))
}

// FeeEstimates reports mode 12: the cost of a payout changed after sending.
//
// The quoted fee is not a leg and moves no value, so it cannot be caught by
// conservation. It has to be compared with what the fee legs actually came to,
// which is all this does.
func FeeEstimates(operations []Operation, tolerance Tolerance) ([]Finding, error) {
	var findings []Finding
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		if operation.FeeEstimate == nil {
			continue
		}
		if finding, ok := feeEstimateFinding(operation, tolerance); ok {
			findings = append(findings, finding)
		}
	}
	return sortFindings(findings), nil
}

// feeEstimateFinding compares one estimate with the fee legs beside it.
func feeEstimateFinding(operation Operation, tolerance Tolerance) (Finding, bool) {
	estimate := operation.FeeEstimate.Abs()
	currency := estimate.Currency()
	actual := feeTotal(operation, currency)

	drift, err := actual.Sub(estimate)
	if err != nil {
		return Finding{}, false
	}
	allowed := tolerance.For(currency)
	if within, _ := drift.Abs().Cmp(allowed); within <= 0 {
		return Finding{}, false
	}

	direction := "more"
	if drift.Sign() < 0 {
		direction = "less"
	}
	summary := fmt.Sprintf("payout cost %s than quoted: estimated %s, charged %s",
		direction, estimate, actual)
	arithmetic := fmt.Sprintf("actual %s - estimate %s = %s, tolerance %s",
		actual.Value(), estimate.Value(), drift.Value(), allowed.Value())

	return newFinding(ModeFeeEstimate, operation.ID, currency, summary, arithmetic,
		evidenceFrom(operation.FeesIn(currency))), true
}
