package reconcile

import (
	"fmt"
	"strconv"
)

// Mode is one of the fourteen failure modes catalogued in the companion
// article. The numbering here is the article's numbering, and it is part of
// the library's interface: a finding is meant to be read next to the entry
// that explains it.
type Mode int

// ModeUnbalanced is the bare conservation violation: the legs of an operation
// do not sum to zero and the shape of the residual matches none of the named
// modes.
//
// It is not one of the fourteen, and it is numbered 00 to say so. The named
// conservation modes — 01, 11, 12 and 14 — are recognised shapes of this one
// failure. An operation that breaks conservation in an unrecognised way is
// still worth reporting rather than dropping because the catalogue has no
// entry for it.
const ModeUnbalanced Mode = 0

// The fourteen. Thirteen are implemented; ModeReferenceRewritten is listed so
// that the gap is visible in the code rather than only in prose.
const (
	ModeFeeConvention       Mode = 1  // More left the account than the payment said.
	ModeDoublePayment       Mode = 2  // The recipient was paid twice.
	ModeAssetWithoutNetwork Mode = 3  // The balance is there and cannot be withdrawn.
	ModeMissingHold         Mode = 4  // The balance went negative despite the check.
	ModeDayBoundary         Mode = 5  // Daily reports do not add up to the monthly one.
	ModeRefundAsNew         Mode = 6  // Turnover doubled out of nowhere.
	ModeRateAsOf            Mode = 7  // One transaction, three different figures.
	ModeReferenceRewritten  Mode = 8  // Yesterday's export names the source differently.
	ModeFinality            Mode = 9  // A confirmed deposit disappeared.
	ModeBatchedTransfer     Mode = 10 // One transaction is not one operation.
	ModeInternalMovement    Mode = 11 // Reconciliation fails though every entry is right.
	ModeFeeEstimate         Mode = 12 // The cost of a payout changed after sending.
	ModeUnwithdrawable      Mode = 13 // The balance exceeds what can be withdrawn.
	ModeFeeInOtherAsset     Mode = 14 // The payout went out and cost nothing.
)

// modeTitles are the article's own titles, kept verbatim so that a reader can
// search for one and land on the entry.
var modeTitles = map[Mode]string{
	ModeUnbalanced:          "The legs of an operation do not sum to zero",
	ModeFeeConvention:       "More left the account than the payment said",
	ModeDoublePayment:       "The recipient was paid twice",
	ModeAssetWithoutNetwork: "The balance is there and cannot be withdrawn",
	ModeMissingHold:         "The balance went negative despite the check",
	ModeDayBoundary:         "Daily reports don't add up to the monthly one",
	ModeRefundAsNew:         "Turnover doubled out of nowhere",
	ModeRateAsOf:            "One transaction, three different figures",
	ModeReferenceRewritten:  "Yesterday's export names the source differently",
	ModeFinality:            "A confirmed deposit disappeared",
	ModeBatchedTransfer:     "One transaction is not one operation",
	ModeInternalMovement:    "Reconciliation fails though every entry is right",
	ModeFeeEstimate:         "The cost of a payout changed after sending",
	ModeUnwithdrawable:      "The balance exceeds what can be withdrawn",
	ModeFeeInOtherAsset:     "The payout went out and cost nothing",
}

// articleBase is where the companion article lives. A finding that cannot be
// followed back to its explanation is half a finding.
const articleBase = "https://jeffreyjorgensen.dev/teardown"

// Number renders the mode as the article prints it: "01" through "14".
func (m Mode) Number() string { return fmt.Sprintf("%02d", int(m)) }

// Title returns the article's title for the mode.
func (m Mode) Title() string {
	if title, ok := modeTitles[m]; ok {
		return title
	}
	return "unknown mode"
}

// Covered reports whether this library detects the mode.
//
// Mode 08 is not covered, and the honest reason is that it cannot be: to see
// that a reference row was rewritten, a detector needs both the old and the
// new state of the table. Given both, it is a diff of two files and teaches
// nothing; given one, it cannot know the row ever changed. The mode is
// prevented by versioning reference data and never updating it in place, which
// is a design rule rather than something a library can retrofit.
func (m Mode) Covered() bool { return m != ModeReferenceRewritten }

// ArticleURL returns the link to this mode's entry in the article. Mode 00 is
// not in the catalogue, so it links to the article itself.
//
// The fragment is the article's own anchor, which is "i" and the mode number
// without a leading zero — "#i4" for mode 04. It is not the number as the
// article prints it, and the difference is the whole value of the link: a
// finding that points at an anchor which does not exist sends the reader to
// the top of a long page and leaves them to find the entry themselves.
func (m Mode) ArticleURL() string {
	if m == ModeUnbalanced {
		return articleBase
	}
	return articleBase + "#i" + strconv.Itoa(int(m))
}

// String renders the mode as "04 — The balance went negative despite the check".
func (m Mode) String() string { return m.Number() + " — " + m.Title() }
