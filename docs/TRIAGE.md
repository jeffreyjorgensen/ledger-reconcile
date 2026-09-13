# Triage: which of the fourteen this library can actually detect

The companion article, *Fourteen Ways Reconciliation Breaks*, numbers fourteen
failure modes. This document decides, one by one, which of them a library can
detect from data it is handed, and states plainly what it needs in order to do
so. The README repeats the conclusion; this file carries the reasoning.

It was written before the code and has been corrected against it. Where the two
would disagree, the code is right and this file is the defect — a test keeps
the verdicts here in step with the coverage table in the README.

The rule applied throughout: a mode is **in scope** only if a synthetic fixture
can reproduce the failure and a negative fixture can be shown not to fire. A
mode that can only be guessed at is out of scope, and saying so is worth more
than a detector that cries wolf.

## Four families, not fourteen detectors

Fourteen independent detectors would be fourteen variations on four ideas, and
nobody reads that in an evening. Each family below asks one question, and the
modes in it are that question asked of different data.

| Family | The question it asks | Modes |
| --- | --- | --- |
| **Conserve** | Is the value accounted for? | 01, 11, 12, 14 |
| **Match** | Is this one event or two? | 02, 06, 10 |
| **Replay** | Was this true at the time? | 04, 05, 07, 09 |
| **Configure** | Do the rules allow what the balance implies? | 03, 13 |

A reader who understands why conservation is checked per currency understands
four entries in the article at once.

## The verdicts

| # | Mode | Verdict | Extra input required |
| --- | --- | --- | --- |
| 01 | Fee on top of the amount, or inside it | **In** | the declared amount, and fee legs |
| 02 | A retried request creates a second payment | **In** | a reference on records; a counterparty and a window for the uncertain half |
| 03 | A currency without its network | **In**, as configuration validation | asset rules, the networks the operator runs |
| 04 | Accounting without holds | **In** | opening balances, and a timestamp on every operation |
| 05 | The day boundary and time zones | **In** | the reporting time zone, and a timestamp on every operation |
| 06 | A refund recorded as a new operation | **In**, narrowly | an explicit link to the operation reversed |
| 07 | A rate as of exactly when | **In** | dated quotes at a positive rate, a counter currency, one principal currency to value from |
| 08 | A reference table edited retroactively | **Out** | — |
| 09 | Finality, not "confirmations" | **In** | a finality rule per chain |
| 10 | Batches, sweeps and asynchronous transfers | **In** | which transaction carried which operations |
| 11 | Internal movements missing from the equation | **In** | external records, and account classification |
| 12 | The actual fee is not the estimated one | **In** | the fee quoted before sending |
| 13 | Dust, minimum reserves and freezes | **In**, as configuration validation | reserves, freezes, minimums |
| 14 | A fee paid in a different asset | **In** | — |

**Thirteen of fourteen, one excluded.** The exclusion is 08, and the reason is
below. The README must say thirteen, never fourteen.

## Mode by mode

### 01 — Fee on top of the amount, or inside it

*Detectable.* The external record debits one figure; the payment declared
another; a fee record claims the difference. The detector asserts
`external_debit == declared_amount + fee` for fee-on-top, and
`external_debit == declared_amount` with `credited == declared_amount - fee`
for fee-inside, then reports which convention the data actually follows. The
failure is rarely a wrong figure; it is two conventions coexisting in one
dataset. So the finding names the convention each record implies.

Needs: the declared amount, and the fee legs of the payment. Without the
declared figure there is nothing for the postings to disagree with, and the
detector skips the payment rather than picking a convention for it.

### 02 — A retried request creates a second payment

*Detectable, in two shapes of unequal certainty.* Two external records carrying
the same reference is a request executed twice, and nothing about it is
ambiguous. The shape that actually happens is a timed-out request retried under
a fresh key: the two payments then share no reference and are alike only in
counterparty, amount and nearness in time. That is a suspicion, and it is
reported as one, in those words.

Needs: a reference on records for the certain half; a counterparty and a window
for the uncertain one. A zero window turns the uncertain half off — how close
is close enough is the caller's judgement, not the library's.

### 03 — A currency without its network

*Not a reconciliation finding — a configuration one.* No arithmetic over
movements reveals it. Given the asset/network table, the check is that every
asset a balance can be held in has at least one network on which it can leave,
and that the network is one the operator actually supports. This ships under
`Configure` and is labelled configuration validation in both the code and the
README. Calling it a reconciliation detector would be a small lie about what
the library does.

Needs: asset rules, and the list of networks the operator actually runs. An
asset that appears in a balance and in no rule is reported too: a balance the
operator holds and has decided nothing about is the state every one of these
failures starts from.

### 04 — Accounting without holds

*Detectable, and the strongest of the set.* This is not a diff between two
sources; it is a replay. Walk the movement stream in order, maintaining
`available` and `held` per account and currency, and report the first point at
which `available` goes negative — together with the movement that caused it and
the hold that should have existed. The finding points at a line in the input,
which is exactly the claim the library exists to demonstrate.

Needs: opening balances. This was the one verdict the implementation
overturned. A replay from an assumed zero reports every account funded before
the window as overdrawn — a sweep out of a treasury that was full yesterday
becomes a finding — and after the fact "went negative" and "started positive
and we were not told" cannot be told apart. So the replay refuses to start
without a stated position for every internal balance it touches; an account
that really began empty is passed as an explicit zero.

For the same reason an operation with no timestamp is refused: this check is
entirely about the order things happened in, and a zero time would sort to the
front silently.

### 05 — The day boundary and time zones

*Detectable and cheap.* Aggregate the same movements under two interpretations
of the day boundary — the reporting zone and UTC — and report every day whose
totals differ, along with the movements that fall in the gap. The output is the
list of transactions that changed days, not merely the statement that totals
differ.

Needs: the reporting time zone. Without it the detector does not run; it does
not quietly assume UTC.

### 06 — A refund recorded as a new operation

*Detectable only with a link.* If a movement declares itself a reversal of
another, the check is arithmetic: turnover must not count both legs. Without
that field the only available approach is a heuristic — same counterparty,
opposite sign, close amount, near in time — and a heuristic that fires on
legitimate round-trip payments is worse than no detector. In a library whose
entire argument is that its findings are defensible, a false finding costs more
than a missed one.

So: implemented against the explicit link, and the README states that the
heuristic was considered and rejected, with this reason.

Needs: an explicit link from the reversal to the operation it undoes. A
reversal whose original is absent is reported as unnettable rather than
ignored, and one that does not mirror what it claims to undo — a partial refund
recorded as a full reversal — is reported separately.

### 07 — A rate as of exactly when

*Detectable.* Given quotes that carry their own timestamps, value the same
operation at the quote in force at each candidate moment — order, execution,
settlement — and report the spread. The finding is the three figures and the
timestamps that produced them, so the reader can see which moment the system
actually used.

Needs: quotes carrying their timestamps, and a currency to express the
valuation in. Only quotes at or before a moment are in force: valuing at a rate
published afterwards would be hindsight dressed up as measurement. A rate at or
below zero is refused rather than used — it is not a price, and a valuation
built on one still produces a spread that looks like an answer.

An exchange is left alone. Valuing it needs one principal currency to value
from, and a trade has two; picking whichever sorts first would mean reasoning
about an arbitrary half of the operation.

### 08 — A reference table edited retroactively — OUT

*Excluded.* To detect it the library would need both the old and the new state
of the reference table. Handed both, the "detector" is a diff of two files and
teaches nothing. Handed one, it cannot know the row ever changed. Neither
version earns its place in a library whose budget is an evening of reading.

The honest statement — which belongs in the README, not buried here — is that
this mode is prevented by how reference data is stored rather than detected
after the fact: rows are versioned and never updated in place, and every export
cites the version it read. That is a design rule, and a library cannot
retrofit it onto a system that did not follow it.

### 09 — Finality, not "confirmations"

*Detectable, and cheaper than the brief assumed.* The extra input is a small
table: chain → the rule under which a deposit is final. The detector reports
every deposit credited before its chain's rule was satisfied, and every deposit
still credited after a reorg removed the transaction that funded it. Two lines
of configuration per chain buys a mode the brief expected to be out of reach.

Needs: a finality rule per chain, of which there are two kinds. Where a chain
reorganises, depth is the only available proxy and the operator picks a number.
Where a chain publishes finality, depth says nothing — a transaction sixty
blocks deep may still be dropped and a shallow finalised one will not be — so
such a chain is judged by what it declared and never by how deep the
transaction sits. A chain with no rule is reported rather than passed.

### 10 — Batches, sweeps and asynchronous transfers

*Detectable.* This is the matching engine itself rather than a detector bolted
onto one: one external transaction against many internal movements, and the
reverse. The finding is the group that failed to balance and the residual
amount, not a complaint about each record in it.

Needs: which external transaction carried which operations. Without that link
there is nothing to group by, and the check has no unit to be about.

### 11 — Internal movements missing from the equation

*Detectable.* The conservation check applied to the whole set: internal
transfers have no external counterpart and must not be expected to have one.
The mode fires when a movement between two internal accounts is counted as if
it crossed the boundary. The finding names the movement and the account pair.

Needs: external records to compare against, and account classification — which
accounts are inside the ledger. A gap that internal movements do not account
for is reported as it stands rather than explained away.

### 12 — The actual fee is not the estimated one

*Detectable.* Compare the estimate recorded at submission with the fee in the
settled record, and report every payout whose difference exceeds a stated
tolerance — with the tolerance carried in the finding, so the reader knows what
was asked rather than only what failed.

Needs: the fee quoted before sending. An unconfigured tolerance means zero: a
check that went quiet because nobody configured it would look like a check that
passed.

### 13 — Dust, minimum reserves and freezes

*Detectable as configuration plus arithmetic.* `withdrawable = available −
frozen − reserve`, never below zero, compared against the smallest amount the
rules allow to be sent. The mode fires when a displayed balance exceeds what
the same inputs say can leave. Like 03, this is mostly a statement about
configuration, and is labelled that way.

Dust has no term of its own. In the article it is one of three reasons a
balance will not move, and in the arithmetic it is the same reason as the
minimum: an amount below the smallest the rules will send. A separate term
would be a figure with nothing different to say.

Needs: reserves, freezes and per-asset minimums.

### 14 — A fee paid in a different asset

*Detectable, and it falls out of `Conserve` for free.* The conservation check
sums the legs of an operation **per currency**. A payout whose fee was paid in
another asset balances in the payout currency and fails to balance in the fee
currency — which is precisely the signal. A checker that summed every leg into
one figure would net the two against each other and see nothing. That is why
the conservation rule is per currency rather than global, and the README says
so, because it is the single least obvious decision in the library.

Needs: nothing beyond the fee records already required by 01.

## What the README carries

The README states the conclusion in three sentences, and the test suite holds
it to them:

1. Thirteen of the fourteen modes are implemented. 08 is not, and the reason is
   that it is prevented by storage design rather than detected after the fact.
2. Several of the thirteen require context the caller must supply and do not
   run without it. None of them falls back to guessing, and a run says which
   checks it could not perform and why.
3. Every implemented mode has a fixture reproducing its failure and a negative
   fixture that must not fire.
