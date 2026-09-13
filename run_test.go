package reconcile

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadInput(t *testing.T, path string) Input {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()

	var input Input
	if err := decoder.Decode(&input); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	return input
}

// The worked example is part of the documentation, so what it produces is
// pinned. A README that showed one output while the code produced another
// would be the same broken promise as claiming an uncovered mode.
func TestRun_WorkedExample_ProducesExactlyTheDocumentedFindings(t *testing.T) {
	report, err := loadInput(t, "testdata/example.json").Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []Mode{
		ModeDoublePayment,    // 02
		ModeMissingHold,      // 04
		ModeFinality,         // 09
		ModeInternalMovement, // 11
		ModeUnwithdrawable,   // 13
	}
	if len(report.Findings) != len(want) {
		t.Fatalf("expected %d findings, got %d:\n%s", len(want), len(report.Findings), report)
	}
	for i, mode := range want {
		if report.Findings[i].Mode != mode {
			t.Errorf("finding %d is mode %s, want %s", i, report.Findings[i].Mode, mode)
		}
	}
}

func TestRun_CleanExample_FindsNothing(t *testing.T) {
	report, err := loadInput(t, "testdata/clean.json").Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("expected nothing, got:\n%s", report)
	}
	if len(report.Ran) < 10 {
		t.Errorf("only %d checks ran on a complete input: %v", len(report.Ran), report.Ran)
	}
}

// A run that quietly performed half the checks and reported "no findings"
// would be the most dangerous output this library could produce.
func TestRun_MissingInput_IsReportedAsNotCheckedNotAsPassed(t *testing.T) {
	report, err := Input{}.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Ran) != 0 {
		t.Errorf("nothing was given, so nothing can have run: %v", report.Ran)
	}
	if len(report.Skipped) != len(checks()) {
		t.Fatalf("every check must account for itself, got %d of %d",
			len(report.Skipped), len(checks()))
	}
	for _, skip := range report.Skipped {
		if skip.Reason == "" {
			t.Errorf("%s was skipped with no reason given", skip.Check)
		}
	}

	rendered := report.String()
	if !strings.Contains(rendered, "Not checked (12)") {
		t.Errorf("the report must lead with how much was not looked at:\n%s", rendered)
	}
}

// The replay is the check most likely to be run on a partial window, and it is
// the one where an assumed starting point invents findings.
func TestRun_OperationsWithoutOpeningBalances_SkipsTheReplayAndSaysWhy(t *testing.T) {
	input := loadInput(t, "testdata/example.json")
	input.Opening = nil

	report, err := input.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found bool
	for _, skip := range report.Skipped {
		if skip.Check != "replay" {
			continue
		}
		found = true
		if !strings.Contains(skip.Reason, "assumed zero") {
			t.Errorf("reason = %q", skip.Reason)
		}
	}
	if !found {
		t.Fatalf("the replay must be reported as skipped, got %+v", report.Skipped)
	}
	for _, finding := range report.Findings {
		if finding.Mode == ModeMissingHold {
			t.Errorf("a skipped check must produce no findings: %s", finding)
		}
	}
}

func TestRun_UnreadableOptions_ReturnError(t *testing.T) {
	cases := []struct {
		name    string
		options Options
		wantHas string
	}{
		{"tolerance", Options{Tolerance: map[Currency]string{usdt: "about ten"}}, "options.tolerance"},
		{"zone", Options{Zone: "Mars/Olympus_Mons"}, "options.zone"},
		{"window", Options{DuplicateWindow: "soon"}, "options.duplicate_window"},
		{"negative window", Options{DuplicateWindow: "-5m"}, "is negative"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Input{Options: c.options}.Run()
			if err == nil {
				t.Fatalf("expected %s to be refused", c.name)
			}
			if !strings.Contains(err.Error(), c.wantHas) {
				t.Errorf("error %q does not mention %q", err, c.wantHas)
			}
		})
	}
}

func TestRun_ErrorFromACheck_NamesWhichOne(t *testing.T) {
	input := loadInput(t, "testdata/clean.json")
	input.Operations[0].Legs[0].Account.Kind = AccountUnknown

	_, err := input.Run()
	if err == nil {
		t.Fatal("expected the malformed operation to be refused")
	}
	if !strings.Contains(err.Error(), "conserve:") {
		t.Errorf("the error must name the check that refused: %v", err)
	}
}

func TestReport_String_WithNoFindings_SaysSoAndStillListsWhatRan(t *testing.T) {
	report, err := loadInput(t, "testdata/clean.json").Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	rendered := report.String()
	if !strings.Contains(rendered, "No findings.") {
		t.Errorf("rendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Checks run (") {
		t.Errorf("a clean report must still say what it looked at:\n%s", rendered)
	}
}

// Named enumerations exist so that a hand-written input file is readable and a
// typo is an error rather than a different meaning.
func TestInput_EnumerationsTravelAsNames(t *testing.T) {
	encoded, err := json.Marshal(Leg{
		ID:      "l1",
		Account: Account{ID: "acct", Kind: AccountSystem},
		Amount:  MustParseAmount("1", usdt),
		Role:    RoleNetworkFee,
		Part:    BalanceHeld,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"kind":"system"`, `"role":"network_fee"`, `"part":"held"`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("encoded leg does not contain %s: %s", want, encoded)
		}
	}

	var leg Leg
	if err := json.Unmarshal(encoded, &leg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if leg.Account.Kind != AccountSystem || leg.Role != RoleNetworkFee || leg.Part != BalanceHeld {
		t.Errorf("round trip changed the leg: %+v", leg)
	}
}

func TestInput_UnknownEnumerationName_IsAnErrorNotADefault(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"account kind", `{"id":"acct","kind":"customer"}`, "not one of user, fee, system, external"},
		{"numeric kind", `{"id":"acct","kind":1}`, "expected a name"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var account Account
			err := json.Unmarshal([]byte(c.body), &account)
			if err == nil {
				t.Fatalf("expected %s to be refused, got %+v", c.name, account)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

// An absent role or part is the common case and has a safe default; an absent
// account kind does not, and must stay an error.
func TestInput_AbsentRoleAndPartDefaultToTheCommonCase(t *testing.T) {
	var leg Leg
	body := `{"id":"l1","account":{"id":"acct","kind":"user"},` +
		`"amount":{"value":"1","currency":"USDT"}}`
	if err := json.Unmarshal([]byte(body), &leg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if leg.Role != RolePrincipal {
		t.Errorf("role = %s, want principal", leg.Role)
	}
	if leg.Part != BalanceAvailable {
		t.Errorf("part = %s, want available", leg.Part)
	}
}
