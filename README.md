# ledger-reconcile

[![ci](https://github.com/jeffreyjorgensen/ledger-reconcile/actions/workflows/ci.yml/badge.svg)](https://github.com/jeffreyjorgensen/ledger-reconcile/actions/workflows/ci.yml)

A reconciliation checker for ledgers that hold other people's money.

It takes what your ledger says happened and what the outside world recorded,
and reports where the two stop agreeing — naming the failure mode, the records
that produced the finding, and the arithmetic that failed.

It is the implementation behind the article
[Fourteen Ways Reconciliation Breaks](https://jeffreyjorgensen.dev/teardown?from=github-ledger).
The article states the problem; this detects it. Every finding carries the
article's own number for the mode it matched, so a finding can be read next to
the prose that explains what it means.

```
[04] acct-user-1 spendable balance fell to -100 USDT: the check that allowed this saw a figure nothing was reserved against
     operation:  op-4-payout
     currency:   USDT
     arithmetic: after op-4-payout: available = -100, held = 0. Covering this would have needed a hold of 100 placed before the earlier operation settled
     leg l1:     acct-user-1 (user, principal) -200 USDT
     see:        https://jeffreyjorgensen.dev/teardown?from=github-ledger#i4
```

**Thirteen of the fourteen modes are implemented.** The fourteenth is not, and
the reason is below rather than buried.

The promise this repository makes is that you can verify it rather than trust
it, so here is what that costs: **2,177 lines of library and command** — the
code you would have to read — and **3,002 lines of tests**, which are the
evidence for it rather than more of it. One dependency outside the standard
library.

## The fourteen

| # | Mode | Detector | What it needs beyond movements |
| --- | --- | --- | --- |
| [01](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i1) | More left the account than the payment said | `FeeConventions` | the declared amount |
| [02](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i2) | The recipient was paid twice | `Duplicates` | a counterparty and a window, for the uncertain half |
| [03](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i3) | The balance is there and cannot be withdrawn | `Configure` | asset rules, supported networks |
| [04](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i4) | The balance went negative despite the check | `Replay` | opening balances |
| [05](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i5) | Daily reports don't add up to the monthly one | `Daily` | the reporting time zone |
| [06](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i6) | Turnover doubled out of nowhere | `Reversals` | a link from the refund to what it reverses |
| [07](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i7) | One transaction, three different figures | `Rates` | dated quotes, a counter currency |
| [08](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i8) | Yesterday's export names the source differently | **not covered** | — |
| [09](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i9) | A confirmed deposit disappeared | `Finality` | a finality rule per chain |
| [10](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i10) | One transaction is not one operation | `Batches` | which transaction carried which operations |
| [11](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i11) | Reconciliation fails though every entry is right | `Boundary` | external records |
| [12](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i12) | The cost of a payout changed after sending | `FeeEstimates` | the fee quoted before sending |
| [13](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i13) | The balance exceeds what can be withdrawn | `Configure` | reserves, freezes, minimums |
| [14](https://jeffreyjorgensen.dev/teardown?from=github-ledger#i14) | The payout went out and cost nothing | `Conserve` | — |

### Why 08 is not covered

To see that a reference row was rewritten, a detector needs both the old and
the new state of the table. Given both, it is a diff of two files and teaches
nothing. Given one, it cannot know the row ever changed.

The mode is prevented rather than detected: reference rows are versioned and
never updated in place, and every export cites the version it read. That is a
design rule, and a library cannot retrofit it onto a system that did not follow
it. Saying so is more useful than a thirteenth detector that only appears to
work.

## Four families, not fourteen detectors

Fourteen independent detectors would be fourteen variations on four ideas. Each
family asks one question, and the modes in it are that question asked of
different data.

| Family | The question it asks | Modes |
| --- | --- | --- |
| **Conserve** | Is the value accounted for? | 01, 11, 12, 14 |
| **Match** | Is this one event or two? | 02, 06, 10 |
| **Replay** | Was this true at the time? | 04, 05, 07, 09 |
| **Configure** | Do the rules allow what the balance implies? | 03, 13 |

The reasoning behind each verdict, including the one the implementation
overturned, is in [docs/TRIAGE.md](docs/TRIAGE.md).

The least obvious decision in the library is that conservation is checked **per
currency** rather than globally. A payout whose network fee was paid in another
asset balances in its settlement currency and is short in the fee currency. A
checker holding one running total would net the two against each other and
report that the operation accounts for itself, which is mode 14 exactly.

## Using it

```go
import reconcile "github.com/jeffreyjorgensen/ledger-reconcile"

findings, err := reconcile.Conserve(operations)
```

Each check is a plain function over plain data. There is no client, no
config file, no daemon, and nothing to start.

To run everything one set of inputs supports at once, hand it all to
[Input.Run]:

```go
input := reconcile.Input{
    Operations: operations,
    Records:    records,
    Opening:    openingBalances,
    Options:    reconcile.Options{Zone: "Europe/Berlin"},
}

report, err := input.Run()
fmt.Print(report) // the findings, then what could not be checked and why
```

### The command

```
go run ./cmd/ledger-reconcile -in testdata/example.json
```

| Exit | Meaning |
| --- | --- |
| 0 | the checks that ran found nothing |
| 1 | at least one finding |
| 2 | the input could not be read, or a check refused to run on it |

One and two are separate codes because a check that could not run is not a
check that passed.

## What it refuses to do

These are the decisions worth knowing before you trust any output.

**It says what it did not check.** A run reports the checks that ran and, for
each one that did not, exactly which input was missing. A tool that quietly
performed four checks out of twelve and printed "no findings" would be worse
than no tool.

```
Checks run (11): conserve, fee-conventions, fee-estimates, replay, reversals,
daily, boundary, batches, finality, duplicates, configure

Not checked (1):
  rates [07] — no rate quotes given
```

**It does not guess a time zone.** `Daily` refuses to run without one.
Assuming UTC would silence the check in exactly the case it exists to find.

**It does not guess a starting balance.** `Replay` refuses to run without
opening balances. Starting from an assumed zero would report every account
funded before the window as overdrawn, and "went negative" and "started
positive and we were not told" are not distinguishable after the fact.

**It does not guess when something happened.** `Replay` and `Daily` refuse an
operation with no timestamp. One is entirely about the order things happened
in, the other about which day they fall on; a zero time would sort to the front
and file itself under the year one, and the answer would be an artefact of the
gap in the data.

**It does not accept a rate that is not a price.** A quote at or below zero is
refused. Used, it would invert or erase a valuation and the spread would still
come out looking like an answer.

**It does not guess finality.** A chain with no rule configured is reported,
not passed. A depth chosen by this library would be a number nobody decided.

**It does not guess a refund.** Mode 06 needs the operation to declare what it
reverses. The heuristic that would replace the link — same counterparty,
opposite sign, close amount, near in time — fires on a customer who pays and is
refunded in the ordinary course of business, and a false finding costs more
here than a missed one.

**It separates certainty from suspicion.** One reference settling twice is a
fact. Two payments that merely look alike are a suspicion, and the finding says
so in those words.

**An unconfigured tolerance means zero.** A check that went quiet because
nobody configured it would look like a check that passed.

**Money is a decimal, everywhere.** There is no constructor that takes a float
and no float anywhere on a path an amount travels, test helpers included.
Amounts refuse to be added across currencies, and refuse to decode from a JSON
number — by the time a value is a JSON number it has already been through a
float. A quantity that will not parse returns an error and never a zero: zero
is a real answer for a minimum, a fee and a balance, and an unreadable field
must not be allowed to impersonate one.

`make money` is what enforces that rule, and this is exactly how far it reaches.
It removes the comment from each line and examines what is left, so a comment
may discuss floats while code may not contain them. It reads text rather than Go
syntax: a float named inside a string literal that itself contains `//` would be
invisible to it. It examines every `.go` file in the tree, so a file the compiler
skips is still checked — an error in that direction rather than the other. An
earlier version discarded any line that carried a comment at all, which meant
`amount float64 // helper` passed it: the bypass was a comment. That is written
down here instead of being left to be discovered, because a check whose reach is
unpublished is a check nobody can weigh — which is this repository's own
argument, and it applies to its tooling first.

**Same input, same findings.** Nothing depends on wall-clock time or on map
iteration order. Findings are sorted by their content before they are returned.

## Input format

One JSON object. Every field is optional; a check whose input is absent does
not run and says so. `testdata/example.json` is a complete worked example, and
what it produces is pinned by a test — a README showing one output while the
code produced another would be the same broken promise as claiming an uncovered
mode.

Amounts travel as strings with their currency. Enumerations travel as names, so
that a hand-written file is readable and a typo is an error rather than a
different meaning:

```json
{
  "id": "l1",
  "account": {"id": "acct-user-1", "kind": "user"},
  "amount": {"value": "-200", "currency": "USDT"},
  "role": "principal"
}
```

`kind` has no default. An account this library had to guess the side of would
be an account its findings could not defend.

## Tests

```
make            # gofmt, vet, and the suite with the race detector
make ci         # the same, plus golangci-lint and the no-float check
make size       # what the figures above are counted from
make example    # the worked example this README opens with
```

Or `go test -race ./...` directly. CI runs those same targets, on the Go
version `go.mod` declares as the floor and on the current one, because a check
that only exists on a server is a check nobody runs while they are working.

152 tests. Every implemented mode has a fixture that reproduces its failure and
a negative fixture that must not fire. All fixtures are synthetic: invented
account ids, round amounts, nothing derived from any real ledger.

The suite is also checked the other way round. Each check was tested by
deliberately breaking it — summing currencies together, counting a fee twice,
valuing a trade at a rate published after the fact, letting a skipped check go
unreported — and confirming the suite went red on each. A green suite that
stays green when the code is wrong is not evidence of anything.

## Dependencies

One: [`shopspring/decimal`](https://github.com/shopspring/decimal), for money.
Everything else is the standard library.

Every dependency is something a reader has to trust before they can trust the
code, and this one buys the single thing the standard library has no answer
for.

## Licence

MIT.
