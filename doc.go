// Package reconcile detects the ways a ledger and the outside world stop
// agreeing with each other.
//
// Each finding names the failure mode it matched by the number used in the
// companion article, "Fourteen Ways Reconciliation Breaks", at
// https://jeffreyjorgensen.dev/teardown, so that a finding can be read next to
// the prose explaining what it means. A finding also carries the records that
// produced it and the arithmetic that failed, because a discrepancy is only
// useful if it points at where to look.
//
// # The checks
//
// Thirteen of the article's fourteen modes are implemented. Each check is a
// plain function over plain data; there is no client to construct and nothing
// to start.
//
//	Conserve        the legs of an operation sum to zero, per currency    (14)
//	FeeConventions  the fee on top of the declared amount, or inside it   (01)
//	FeeEstimates    what was quoted before sending against what was paid  (12)
//	Boundary        what crossed the ledger's edge against what was seen  (11)
//	Replay          a spendable balance driven below zero, in time order  (04)
//	Daily           the reporting day boundary against UTC                (05)
//	Rates           one operation valued at each moment it passed         (07)
//	Finality        a credit made before the chain's rule was satisfied   (09)
//	Duplicates      one request settled twice, certainly or probably      (02)
//	Batches         one transaction against the group it settled          (10)
//	Reversals       a refund counted as a fresh operation                 (06)
//	Configure       an asset with nowhere to go, or a balance stuck       (03, 13)
//
// Input.Run performs every check its input supports and reports, for each one
// it could not perform, which input was missing. A run that quietly performed
// half the checks would make the same false claim as a README promising
// coverage it does not have.
//
// Mode 08 — a reference table edited retroactively — is not implemented.
// Detecting it needs both the old and the new state of the table: given both,
// it is a diff of two files; given one, it cannot know the row ever changed.
// The mode is prevented by versioning reference data rather than detected
// afterwards, and a library cannot retrofit that onto a system that did not do
// it.
//
// # Money
//
// Amounts are decimal and carry their currency. They refuse to be added across
// currencies, refuse to decode from a JSON number, and return an error rather
// than a zero when a quantity will not parse — zero is a real answer for a
// minimum, a fee and a balance, and an unreadable field must not be allowed to
// impersonate one.
//
// Conservation is checked per currency rather than globally, which is the
// least obvious decision here. A payout whose network fee was paid in another
// asset balances in its settlement currency and is short in the fee currency;
// a single running total would net the two against each other and report that
// the operation accounts for itself. That is mode 14 exactly.
//
// # Determinism
//
// Same input, same findings. Nothing depends on wall-clock time or on map
// iteration order, and findings are sorted by their content before they are
// returned.
package reconcile
