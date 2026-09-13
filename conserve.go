package reconcile

import "fmt"

// Conserve checks that the legs of every operation sum to zero in each
// currency separately.
//
// Per currency is the whole point. A single total across currencies would let
// a payout in one asset and a fee in another cancel each other out and report
// that the operation accounts for itself, which is mode 14 — the payout that
// went out and appeared to cost nothing. This is the least obvious decision in
// the library, and everything else in this file follows from it.
//
// The returned findings are sorted and do not depend on map iteration order.
func Conserve(operations []Operation) ([]Finding, error) {
	var findings []Finding
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		findings = append(findings, conserveOne(operation)...)
	}
	return sortFindings(findings), nil
}

// conserveOne reports every currency in which one operation fails to balance.
func conserveOne(operation Operation) []Finding {
	residuals := operation.Sum().NonZero()
	if len(residuals) == 0 {
		return nil
	}

	findings := make([]Finding, 0, len(residuals))
	for _, residual := range residuals {
		findings = append(findings, classifyResidual(operation, residual))
	}
	return findings
}

// classifyResidual decides which mode a single unbalanced currency represents.
//
// Mode 14 is a narrow shape and the test for it has to be equally narrow: the
// short currency must carry fees and no principal at all, and the operation
// must settle — balance — in a currency that does carry principal. That is the
// case the mode describes: a payout whose cost was taken in an asset the
// payout itself never moved, so a check in the settlement currency reports
// nothing.
//
// Anything looser mislabels. A trade is denominated in two currencies and
// neither is "a different asset"; an operation with no principal legs has no
// settlement currency for the mode to be invisible in.
func classifyResidual(operation Operation, residual Amount) Finding {
	currency := residual.Currency()
	fees := operation.FeesIn(currency)

	if len(fees) > 0 && !operation.HasPrincipalIn(currency) {
		if settled, ok := settlementCurrency(operation); ok {
			return feeInOtherAssetFinding(operation, residual, settled, fees)
		}
	}
	return unbalancedFinding(operation, residual)
}

// settlementCurrency returns a principal currency the operation balances in.
//
// Mode 14 is only worth the name when such a currency exists: it is the one a
// check would have looked at and found nothing wrong.
func settlementCurrency(operation Operation) (Currency, bool) {
	total := operation.Sum()
	for _, currency := range operation.PrincipalCurrencies() {
		if total.In(currency).IsZero() {
			return currency, true
		}
	}
	return "", false
}

// feeInOtherAssetFinding reports mode 14: the operation settles in its own
// currency, so every check denominated in that currency says it is fine, while
// the asset the fee was actually paid in is short.
func feeInOtherAssetFinding(operation Operation, residual Amount, principal Currency, fees []Leg) Finding {
	summary := fmt.Sprintf(
		"operation settles in %s but is short %s: the fee was paid in a different asset and nothing accounts for it",
		principal, residual.Abs())

	// The second half of the arithmetic is the reason the mode is hard to see
	// in production: the settlement currency balances, so a checker that looks
	// only there reports nothing. State that total rather than assert it.
	inPrincipal := operation.Sum().In(principal)
	arithmetic := fmt.Sprintf(
		"sum(legs in %s) = %s, expected 0; meanwhile sum(legs in %s) = %s, which is why a check in %s alone reports nothing",
		residual.Currency(), residual.Value().String(),
		currencyOrDash(principal), inPrincipal.Value().String(), currencyOrDash(principal))

	return newFinding(ModeFeeInOtherAsset, operation.ID, residual.Currency(),
		summary, arithmetic, evidenceFrom(fees))
}

// unbalancedFinding reports mode 00: value was created or destroyed and no
// named mode explains how.
func unbalancedFinding(operation Operation, residual Amount) Finding {
	direction := "more value arrived than left"
	if residual.Sign() < 0 {
		direction = "more value left than arrived"
	}

	summary := fmt.Sprintf("legs do not sum to zero in %s: %s", residual.Currency(), direction)
	arithmetic := fmt.Sprintf("sum(legs in %s) = %s, expected 0",
		residual.Currency(), residual.Value().String())

	return newFinding(ModeUnbalanced, operation.ID, residual.Currency(),
		summary, arithmetic, evidenceFrom(legsIn(operation, residual.Currency())))
}

// currencyOrDash keeps the arithmetic line readable when a currency is absent.
func currencyOrDash(currency Currency) string {
	if currency == "" {
		return "—"
	}
	return string(currency)
}
