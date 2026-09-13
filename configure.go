package reconcile

import (
	"fmt"
	"sort"
)

// AssetRule is everything the operator has decided about one asset.
//
// It is configuration, not observation, which is why the two modes in this
// file are labelled configuration validation rather than reconciliation. No
// amount of arithmetic over movements reveals that an asset has nowhere to go.
type AssetRule struct {
	// Networks are the networks this asset can leave on. An asset with none is
	// mode 03: a balance that can be credited and can never be withdrawn.
	Networks []string `json:"networks,omitempty"`

	// MinimumWithdrawal is the smallest amount that may be sent out.
	MinimumWithdrawal Amount `json:"minimum_withdrawal,omitempty"`

	// Reserve is what must remain in the account and can never be withdrawn.
	Reserve Amount `json:"reserve,omitempty"`
}

// AssetRules maps a currency to the rule governing it.
type AssetRules map[Currency]AssetRule

// Configure reports modes 03 and 13.
//
// Both are statements about configuration rather than about movements, and
// both produce the same complaint from a customer — the balance is there and
// will not come out — from two unrelated causes. The first is an asset with no
// network to leave on. The second is an amount smaller than the rules allow to
// move, once reserves and freezes are taken off.
//
// An asset that appears in a balance and in no rule is reported. A balance the
// operator holds and has decided nothing about is the state every one of these
// failures starts from.
func Configure(balances []Balance, rules AssetRules, supported []string) ([]Finding, error) {
	networks := map[string]bool{}
	for _, network := range supported {
		networks[network] = true
	}

	var findings []Finding
	for _, balance := range sortedBalances(balances) {
		if err := balance.validate(); err != nil {
			return nil, err
		}
		currency := balance.Available.Currency()
		rule, configured := rules[currency]

		if !configured {
			findings = append(findings, unconfiguredAssetFinding(balance))
			continue
		}
		if finding, ok := networkFinding(balance, rule, networks); ok {
			findings = append(findings, finding)
		}
		if finding, ok := withdrawableFinding(balance, rule); ok {
			findings = append(findings, finding)
		}
	}
	return sortFindings(findings), nil
}

// withdrawable is what can actually leave: the spendable figure less what is
// frozen and what must stay behind, never below zero.
func (b Balance) withdrawable(rule AssetRule) Amount {
	currency := b.Available.Currency()
	result := b.Available

	for _, deduction := range []Amount{b.Frozen, rule.Reserve} {
		if deduction.Currency() != currency {
			continue
		}
		reduced, err := result.Sub(deduction.Abs())
		if err != nil {
			continue
		}
		result = reduced
	}
	if result.Sign() < 0 {
		return zeroAmount(currency)
	}
	return result
}

// unconfiguredAssetFinding reports a balance in an asset nothing describes.
func unconfiguredAssetFinding(balance Balance) Finding {
	currency := balance.Available.Currency()
	summary := fmt.Sprintf("%s holds %s and no rule describes that asset",
		balance.Account.ID, balance.Available)
	arithmetic := fmt.Sprintf(
		"no entry for %s: nothing says which networks it can leave on, what must stay behind, or what the minimum is",
		currency)

	return newFinding(ModeAssetWithoutNetwork, balance.Account.ID, currency, summary, arithmetic, nil)
}

// networkFinding reports mode 03: the asset cannot leave.
func networkFinding(balance Balance, rule AssetRule, supported map[string]bool) (Finding, bool) {
	currency := balance.Available.Currency()

	if len(rule.Networks) == 0 {
		summary := fmt.Sprintf("%s can be credited in %s and has no network to leave on",
			balance.Account.ID, currency)
		arithmetic := fmt.Sprintf("%s: balance %s, networks configured: none",
			currency, balance.Available.Value())
		return newFinding(ModeAssetWithoutNetwork, balance.Account.ID, currency,
			summary, arithmetic, nil), true
	}

	for _, network := range rule.Networks {
		if supported[network] {
			return Finding{}, false
		}
	}

	summary := fmt.Sprintf("%s can leave only on %v, none of which the operator supports",
		currency, rule.Networks)
	arithmetic := fmt.Sprintf("%s: balance %s, networks %v, supported %v",
		currency, balance.Available.Value(), rule.Networks, sortedNetworks(supported))
	return newFinding(ModeAssetWithoutNetwork, balance.Account.ID, currency,
		summary, arithmetic, nil), true
}

// withdrawableFinding reports mode 13: the shown balance exceeds what can go.
func withdrawableFinding(balance Balance, rule AssetRule) (Finding, bool) {
	currency := balance.Available.Currency()
	if balance.Available.Sign() <= 0 {
		return Finding{}, false
	}

	canLeave := balance.withdrawable(rule)
	minimum := rule.MinimumWithdrawal.Abs()
	if minimum.Currency() != currency {
		minimum = zeroAmount(currency)
	}
	if enough, _ := canLeave.Cmp(minimum); enough >= 0 && !canLeave.IsZero() {
		return Finding{}, false
	}

	summary := fmt.Sprintf(
		"%s shows %s and can withdraw %s: every part of the difference has a name — "+
			"reserve, freeze, and a remainder below the minimum — and none of them is a "+
			"missing entry",
		balance.Account.ID, balance.Available, canLeave)
	arithmetic := fmt.Sprintf(
		"available %s - frozen %s - reserve %s = %s, below the minimum of %s",
		balance.Available.Value(), amountOrZero(balance.Frozen, currency).Value(),
		amountOrZero(rule.Reserve, currency).Value(), canLeave.Value(), minimum.Value())

	return newFinding(ModeUnwithdrawable, balance.Account.ID, currency, summary, arithmetic, nil), true
}

// sortedBalances orders balances so the output does not depend on the caller's
// ordering.
func sortedBalances(balances []Balance) []Balance {
	ordered := make([]Balance, len(balances))
	copy(ordered, balances)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Account.ID != ordered[j].Account.ID {
			return ordered[i].Account.ID < ordered[j].Account.ID
		}
		return ordered[i].Available.Currency() < ordered[j].Available.Currency()
	})
	return ordered
}

// sortedNetworks lists the supported networks in a stable order.
func sortedNetworks(supported map[string]bool) []string {
	networks := make([]string, 0, len(supported))
	for network := range supported {
		networks = append(networks, network)
	}
	sort.Strings(networks)
	return networks
}
