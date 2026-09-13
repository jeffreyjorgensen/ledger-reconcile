package reconcile

import (
	"strings"
	"testing"
)

// A finding is only worth anything if a reader can act on it without the input
// file open beside them. This pins the parts that make that true.
func TestFinding_String_CarriesTheArithmeticTheEvidenceAndTheLink(t *testing.T) {
	findings, err := Conserve([]Operation{withdrawalFeeInTRX()})
	if err != nil {
		t.Fatalf("Conserve: %v", err)
	}

	rendered := findings[0].String()
	for _, want := range []string{
		"[14]",                    // the mode, as the article numbers it
		"op-withdraw-fee-trx",     // where it happened
		"TRX",                     // in which currency
		"sum(legs in TRX) = -0.5", // the arithmetic, checkable by hand
		"acct-treasury",           // the record that produced it
		"network fee",             // in what role
		"/teardown#i14",           // where the explanation lives
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered finding does not contain %q:\n%s", want, rendered)
		}
	}
}

func TestSortFindings_OrderDependsOnContentAlone(t *testing.T) {
	findings := []Finding{
		{Mode: ModeFeeInOtherAsset, Operation: "op-b", Currency: trx, Summary: "z"},
		{Mode: ModeUnbalanced, Operation: "op-b", Currency: btc, Summary: "a"},
		{Mode: ModeFeeInOtherAsset, Operation: "op-a", Currency: trx, Summary: "a"},
		{Mode: ModeFeeInOtherAsset, Operation: "op-b", Currency: btc, Summary: "a"},
		{Mode: ModeFeeInOtherAsset, Operation: "op-b", Currency: trx, Summary: "a"},
	}

	sorted := sortFindings(findings)

	want := []string{
		"00/op-b/BTC/a",
		"14/op-a/TRX/a",
		"14/op-b/BTC/a",
		"14/op-b/TRX/a",
		"14/op-b/TRX/z",
	}
	for i, expected := range want {
		got := sorted[i].Mode.Number() + "/" + sorted[i].Operation + "/" +
			string(sorted[i].Currency) + "/" + sorted[i].Summary
		if got != expected {
			t.Errorf("position %d = %s, want %s", i, got, expected)
		}
	}
}

func TestMode_String_LeadsWithTheArticleNumber(t *testing.T) {
	if got := ModeMissingHold.String(); got != "04 — The balance went negative despite the check" {
		t.Errorf("String() = %q", got)
	}
	if got := ModeUnbalanced.ArticleURL(); got != articleBase {
		t.Errorf("mode 00 is not in the catalogue and must link to the article itself, got %q", got)
	}
	if got := Mode(99).Title(); got != "unknown mode" {
		t.Errorf("Title() for an unknown mode = %q", got)
	}
}

// The anchors are the article's, not a guess at them, and the difference is
// not cosmetic: "#04" is what the article prints beside the entry, "#i4" is
// what it names the entry. Every finding carried a link to the first form
// until the page was read, and every one of them landed at the top of a long
// page with the reader left to search it.
func TestMode_ArticleURL_UsesTheArticlesOwnAnchors(t *testing.T) {
	cases := map[Mode]string{
		ModeFeeConvention:   "https://jeffreyjorgensen.dev/teardown#i1",
		ModeMissingHold:     "https://jeffreyjorgensen.dev/teardown#i4",
		ModeFinality:        "https://jeffreyjorgensen.dev/teardown#i9",
		ModeBatchedTransfer: "https://jeffreyjorgensen.dev/teardown#i10",
		ModeFeeInOtherAsset: "https://jeffreyjorgensen.dev/teardown#i14",
	}

	for mode, want := range cases {
		if got := mode.ArticleURL(); got != want {
			t.Errorf("mode %s links to %q, want %q", mode.Number(), got, want)
		}
	}

	// The number the article prints and the anchor it uses are different
	// strings, and mode 04 is where a leading zero would slip in unnoticed.
	if strings.Contains(ModeMissingHold.ArticleURL(), "#i04") {
		t.Error("the anchor carries no leading zero")
	}
}
