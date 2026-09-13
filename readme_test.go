package reconcile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The README makes four checkable claims: which modes are covered, which one
// is not, how large the thing is, and how many tests there are. Those claims
// are the argument this repository exists to make, so they are checked here
// rather than maintained by hand and hoped over.
//
// When one of these fails, the README is the thing to correct.

func readREADME(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	return string(body)
}

var modeRowPattern = regexp.MustCompile(`\| \[(\d\d)\]\([^)]+\) \| [^|]+ \| ([^|]+) \|`)

// modesInREADME reads the table and returns, for each mode number, whether the
// README claims it is covered.
func modesInREADME(t *testing.T, readme string) map[Mode]bool {
	t.Helper()

	claimed := map[Mode]bool{}
	for _, row := range modeRowPattern.FindAllStringSubmatch(readme, -1) {
		number, err := strconv.Atoi(row[1])
		if err != nil {
			t.Fatalf("mode number %q in the README table: %v", row[1], err)
		}
		status := strings.TrimSpace(row[2])
		claimed[Mode(number)] = status != "**not covered**"
	}
	return claimed
}

// modesWithDetectors reads the source and returns every mode that actually
// produces a finding.
func modesWithDetectors(t *testing.T) map[Mode]bool {
	t.Helper()

	byName := map[string]Mode{}
	for mode := ModeFeeConvention; mode <= ModeFeeInOtherAsset; mode++ {
		byName[modeConstantNames[mode]] = mode
	}

	produced := map[Mode]bool{}
	pattern := regexp.MustCompile(`newFinding\((Mode[A-Za-z]+)`)
	for _, path := range goFiles(t, false) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
			if mode, known := byName[match[1]]; known {
				produced[mode] = true
			}
		}
	}
	return produced
}

// modeConstantNames ties each mode to the identifier the detectors use, so the
// source can be read for which modes are live.
var modeConstantNames = map[Mode]string{
	ModeFeeConvention:       "ModeFeeConvention",
	ModeDoublePayment:       "ModeDoublePayment",
	ModeAssetWithoutNetwork: "ModeAssetWithoutNetwork",
	ModeMissingHold:         "ModeMissingHold",
	ModeDayBoundary:         "ModeDayBoundary",
	ModeRefundAsNew:         "ModeRefundAsNew",
	ModeRateAsOf:            "ModeRateAsOf",
	ModeReferenceRewritten:  "ModeReferenceRewritten",
	ModeFinality:            "ModeFinality",
	ModeBatchedTransfer:     "ModeBatchedTransfer",
	ModeInternalMovement:    "ModeInternalMovement",
	ModeFeeEstimate:         "ModeFeeEstimate",
	ModeUnwithdrawable:      "ModeUnwithdrawable",
	ModeFeeInOtherAsset:     "ModeFeeInOtherAsset",
}

// "Do not claim fourteen if you ship ten" as something the suite enforces
// rather than something the author remembers.
func TestREADME_CoverageTableMatchesTheCode(t *testing.T) {
	claimed := modesInREADME(t, readREADME(t))
	produced := modesWithDetectors(t)

	if len(claimed) != 14 {
		t.Fatalf("the README table lists %d modes, want 14", len(claimed))
	}

	for mode := ModeFeeConvention; mode <= ModeFeeInOtherAsset; mode++ {
		says, listed := claimed[mode]
		if !listed {
			t.Errorf("mode %s is missing from the README table", mode.Number())
			continue
		}
		switch {
		case says && !produced[mode]:
			t.Errorf("the README claims mode %s is covered, and no detector produces it",
				mode.Number())
		case !says && produced[mode]:
			t.Errorf("mode %s is detected and the README does not say so", mode.Number())
		}
		if says != mode.Covered() {
			t.Errorf("mode %s: README says covered=%v, Mode.Covered() says %v",
				mode.Number(), says, mode.Covered())
		}
	}
}

// goFiles lists the package's Go files, tests or not.
func goFiles(t *testing.T, tests bool) []string {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") == tests {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	return paths
}

// countLines counts lines that are neither blank nor comment-only. Comments
// are excluded because the budget is about how much code a stranger has to
// verify, and prose explaining the code is not that.
func countLines(t *testing.T, paths []string) int {
	t.Helper()

	total := 0
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "//") {
				continue
			}
			total++
		}
	}
	return total
}

// The promise on the site is that the code can be verified line by line. The
// figure in the README is the reader's estimate of what that costs them, so it
// has to be the real one.
func TestREADME_StatedSizeIsTheRealSize(t *testing.T) {
	readme := readREADME(t)

	library := countLines(t, goFiles(t, false))
	tests := countLines(t, goFiles(t, true))

	// The budget is against the library and command, because that is what the
	// promise is about: the code a reader has to verify before trusting the
	// answers. The tests are the evidence for those answers — sampled, not
	// audited line by line — and their size is stated beside it rather than
	// folded into it, so nothing is hidden by the choice.
	const budget = 5000
	if library > budget {
		t.Errorf("%d lines of library and command, over the stated budget of %d", library, budget)
	}

	for _, claim := range []struct {
		what  string
		count int
	}{
		{"library and command", library},
		{"tests", tests},
	} {
		stated := formatThousands(claim.count)
		if !strings.Contains(readme, stated) {
			t.Errorf("the README does not state %s as %s lines of %s",
				stated, stated, claim.what)
		}
	}
}

// countTests counts the test functions the suite actually defines.
func countTests(t *testing.T) int {
	t.Helper()

	pattern := regexp.MustCompile(`(?m)^func Test[A-Za-z0-9_]*\(`)
	total := 0
	for _, path := range goFiles(t, true) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		total += len(pattern.FindAllString(string(body), -1))
	}
	return total
}

func TestREADME_StatedTestCountIsTheRealCount(t *testing.T) {
	readme := readREADME(t)
	count := countTests(t)

	if !strings.Contains(readme, fmt.Sprintf("%d tests", count)) {
		t.Errorf("the README does not say %d tests", count)
	}
}

// formatThousands renders 2085 as "2,085", which is how the README reads.
func formatThousands(n int) string {
	digits := strconv.Itoa(n)
	if len(digits) <= 3 {
		return digits
	}
	head := len(digits) % 3
	if head == 0 {
		head = 3
	}
	out := digits[:head]
	for i := head; i < len(digits); i += 3 {
		out += "," + digits[i:i+3]
	}
	return out
}

var triageRowPattern = regexp.MustCompile(`(?m)^\| (\d\d) \| [^|]+ \| ([^|]+) \|`)

// The triage document was written before the code and is the easiest thing in
// the repository to leave behind. A reader who found it contradicting the
// README would be right to stop reading.
func TestTriage_VerdictsAgreeWithTheREADME(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("docs", "TRIAGE.md"))
	if err != nil {
		t.Fatalf("reading docs/TRIAGE.md: %v", err)
	}

	verdicts := map[Mode]bool{}
	for _, row := range triageRowPattern.FindAllStringSubmatch(string(body), -1) {
		number, err := strconv.Atoi(row[1])
		if err != nil {
			t.Fatalf("mode number %q in the triage table: %v", row[1], err)
		}
		verdict := strings.TrimSpace(row[2])
		switch {
		case strings.HasPrefix(verdict, "**In**"):
			verdicts[Mode(number)] = true
		case strings.HasPrefix(verdict, "**Out**"):
			verdicts[Mode(number)] = false
		default:
			t.Fatalf("mode %s carries the unreadable verdict %q", row[1], verdict)
		}
	}

	if len(verdicts) != 14 {
		t.Fatalf("the triage table covers %d modes, want 14", len(verdicts))
	}
	for mode, in := range verdicts {
		if in != mode.Covered() {
			t.Errorf("triage says mode %s is in=%v, the code says covered=%v",
				mode.Number(), in, mode.Covered())
		}
	}
}

// Every covered mode belongs to exactly one family, and the two documents must
// put it in the same one.
func TestTriage_FamilyTableMatchesTheREADME(t *testing.T) {
	triage, err := os.ReadFile(filepath.Join("docs", "TRIAGE.md"))
	if err != nil {
		t.Fatalf("reading docs/TRIAGE.md: %v", err)
	}

	inTriage := familyTable(t, string(triage))
	inREADME := familyTable(t, readREADME(t))

	if len(inTriage) != 13 {
		t.Errorf("the triage family table places %d modes, want 13", len(inTriage))
	}
	for mode, family := range inREADME {
		if other, placed := inTriage[mode]; !placed || other != family {
			t.Errorf("mode %s is in %q in the README and %q in the triage",
				mode.Number(), family, other)
		}
	}
	for mode := ModeFeeConvention; mode <= ModeFeeInOtherAsset; mode++ {
		_, placed := inREADME[mode]
		if placed != mode.Covered() {
			t.Errorf("mode %s: placed in a family=%v, covered=%v",
				mode.Number(), placed, mode.Covered())
		}
	}
}

var familyRowPattern = regexp.MustCompile(`(?m)^\| \*\*(Conserve|Match|Replay|Configure)\*\* \| [^|]+ \| ([0-9, ]+) \|`)

// familyTable reads "| **Conserve** | … | 01, 11, 12, 14 |" rows.
func familyTable(t *testing.T, document string) map[Mode]string {
	t.Helper()

	placed := map[Mode]string{}
	for _, row := range familyRowPattern.FindAllStringSubmatch(document, -1) {
		for _, number := range strings.Split(row[2], ",") {
			value, err := strconv.Atoi(strings.TrimSpace(number))
			if err != nil {
				t.Fatalf("mode number %q in a family table: %v", number, err)
			}
			if previous, twice := placed[Mode(value)]; twice {
				t.Errorf("mode %02d is in both %q and %q", value, previous, row[1])
			}
			placed[Mode(value)] = row[1]
		}
	}
	return placed
}

// The README opens with a finding, and a reader's first act is to run the tool
// and see whether it produces that. Anything less than the literal output
// would be a small dishonesty on the most visible line of the repository.
func TestREADME_OpeningExampleIsLiteralOutput(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}

	start := strings.Index(string(readme), "[04] ")
	if start < 0 {
		t.Fatal("the README no longer opens with a worked finding")
	}
	end := strings.Index(string(readme)[start:], "\n```")
	if end < 0 {
		t.Fatal("the README's opening example is not a closed code block")
	}
	quoted := string(readme)[start : start+end]

	report, err := loadInput(t, "testdata/example.json").Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(report.String(), quoted) {
		t.Errorf("the README quotes an example the code does not produce.\nquoted:\n%s\n\nproduced:\n%s",
			quoted, report)
	}
}
