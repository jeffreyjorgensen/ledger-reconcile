package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	exampleInput = "../../testdata/example.json"
	cleanInput   = "../../testdata/clean.json"
)

// Findings and "could not be checked" are different outcomes from "clean", and
// a caller wiring this into a pipeline needs to tell them apart without
// reading the text.
//
// The expected codes are written out as numbers rather than as the constants
// they came from. A test that compared the constants against themselves would
// pass however they were later redefined, which is exactly what it exists to
// prevent: these three numbers are a promise made to whatever runs this.
func TestRun_ExitCodes(t *testing.T) {
	cases := []struct {
		name string
		path string
		want int
	}{
		{"clean", cleanInput, 0},
		{"findings", exampleInput, 1},
		{"no such file", filepath.Join(t.TempDir(), "absent.json"), 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(c.path, false, &stdout, &stderr); got != c.want {
				t.Errorf("exit = %d, want %d (stderr: %s)", got, c.want, stderr.String())
			}
		})
	}
}

func TestRun_TextOutput_ShowsFindingsAndWhatWasNotChecked(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(exampleInput, false, &stdout, &stderr); code != exitFindings {
		t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"5 finding(s):",
		"[04]", "[09]", "[11]", "[13]", "[02]",
		"https://jeffreyjorgensen.dev/teardown#i4",
		"Checks run (11):",
		"Not checked (1):",
		"rates [07] — no rate quotes given",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestRun_JSONOutput_IsValidAndCarriesTheSameCounts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(exampleInput, true, &stdout, &stderr); code != exitFindings {
		t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
	}

	var report struct {
		Findings []struct {
			Mode      int    `json:"mode"`
			Article   string `json:"article"`
			Operation string `json:"operation"`
		} `json:"findings"`
		Ran     []string `json:"ran"`
		Skipped []struct {
			Check  string `json:"check"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}

	if len(report.Findings) != 5 {
		t.Errorf("findings = %d, want 5", len(report.Findings))
	}
	if len(report.Ran) != 11 {
		t.Errorf("ran = %d, want 11", len(report.Ran))
	}
	if len(report.Skipped) != 1 || report.Skipped[0].Check != "rates" {
		t.Errorf("skipped = %+v", report.Skipped)
	}
	for _, finding := range report.Findings {
		if finding.Article == "" {
			t.Errorf("finding %+v carries no link to its explanation", finding)
		}
	}
}

// A misspelled field would otherwise become a check that silently had nothing
// to work on, and the run would report fewer findings while looking healthy.
func TestRun_UnknownFieldInInput_IsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typo.json")
	body := `{"operatoins": [], "options": {"zone": "UTC"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(path, false, &stdout, &stderr); code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "operatoins") {
		t.Errorf("the error must name the field it did not recognise: %s", stderr.String())
	}
}

func TestRun_MalformedJSON_IsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte(`{"operations": [`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(path, false, &stdout, &stderr); code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "reading input") {
		t.Errorf("stderr = %s", stderr.String())
	}
}

// The zone database is embedded, so the day-boundary check behaves the same on
// a machine that carries no system copy of it.
func TestRun_NamedTimeZone_ResolvesWithoutTheSystemDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zoned.json")
	body := `{"options": {"zone": "Australia/Eucla"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(path, false, &stdout, &stderr); code != exitClean {
		t.Fatalf("exit = %d, stderr: %s", code, stderr.String())
	}
}

func TestRun_UnreadableOption_ExitsWithTheErrorCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-zone.json")
	body := `{"options": {"zone": "Mars/Olympus_Mons"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(path, false, &stdout, &stderr); code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "options.zone") {
		t.Errorf("stderr = %s", stderr.String())
	}
}
