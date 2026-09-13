package reconcile

import (
	"encoding/json"
	"fmt"
)

// The three enumerations in the model travel through JSON as their names
// rather than their numbers.
//
// Input files are written and read by people. A number would make the most
// consequential field in the model — whether an account is inside the ledger
// or outside it — a digit that is easy to mistype and impossible to read back,
// and the zero value would quietly mean "unclassified" for anyone who left it
// out. Names make a wrong value an error at the point it is written.

var accountKindNames = map[AccountKind]string{
	AccountUser:     "user",
	AccountFee:      "fee",
	AccountSystem:   "system",
	AccountExternal: "external",
}

var legRoleNames = map[LegRole]string{
	RolePrincipal:  "principal",
	RoleFee:        "fee",
	RoleNetworkFee: "network_fee",
	RoleAdjustment: "adjustment",
}

var balancePartNames = map[BalancePart]string{
	BalanceAvailable: "available",
	BalanceHeld:      "held",
}

// MarshalJSON writes the account kind as its name.
func (k AccountKind) MarshalJSON() ([]byte, error) {
	name, ok := accountKindNames[k]
	if !ok {
		return nil, fmt.Errorf("account kind: %d is not a kind", int(k))
	}
	return json.Marshal(name)
}

// UnmarshalJSON reads an account kind by name. There is no default: an account
// this library had to guess the side of is an account its findings could not
// defend.
func (k *AccountKind) UnmarshalJSON(data []byte) error {
	name, err := decodeName(data, "account kind")
	if err != nil {
		return err
	}
	for kind, known := range accountKindNames {
		if known == name {
			*k = kind
			return nil
		}
	}
	return fmt.Errorf("account kind: %q is not one of user, fee, system, external", name)
}

// MarshalJSON writes the leg role as its name.
func (r LegRole) MarshalJSON() ([]byte, error) {
	name, ok := legRoleNames[r]
	if !ok {
		return nil, fmt.Errorf("leg role: %d is not a role", int(r))
	}
	return json.Marshal(name)
}

// UnmarshalJSON reads a leg role by name, defaulting to principal when the
// field is absent. Most legs are principal, and a leg wrongly called principal
// costs a fee comparison; a leg wrongly called a fee would be double-counted
// against the payer, which is worse.
func (r *LegRole) UnmarshalJSON(data []byte) error {
	name, err := decodeName(data, "leg role")
	if err != nil {
		return err
	}
	if name == "" {
		*r = RolePrincipal
		return nil
	}
	for role, known := range legRoleNames {
		if known == name {
			*r = role
			return nil
		}
	}
	return fmt.Errorf("leg role: %q is not one of principal, fee, network_fee, adjustment", name)
}

// MarshalJSON writes the balance part as its name.
func (p BalancePart) MarshalJSON() ([]byte, error) {
	name, ok := balancePartNames[p]
	if !ok {
		return nil, fmt.Errorf("balance part: %d is not a part", int(p))
	}
	return json.Marshal(name)
}

// UnmarshalJSON reads a balance part by name, defaulting to available when the
// field is absent, which is what most legs move.
func (p *BalancePart) UnmarshalJSON(data []byte) error {
	name, err := decodeName(data, "balance part")
	if err != nil {
		return err
	}
	if name == "" {
		*p = BalanceAvailable
		return nil
	}
	for part, known := range balancePartNames {
		if known == name {
			*p = part
			return nil
		}
	}
	return fmt.Errorf("balance part: %q is not one of available, held", name)
}

// decodeName reads a JSON string, rejecting a number so that a file written
// against an older numeric form fails loudly instead of meaning something else.
func decodeName(data []byte, what string) (string, error) {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return "", fmt.Errorf("%s: expected a name, got %s", what, string(data))
	}
	return name, nil
}
