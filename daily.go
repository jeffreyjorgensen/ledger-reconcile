package reconcile

import (
	"fmt"
	"sort"
	"time"
)

const dateLayout = "2006-01-02"

// Daily reports mode 05: daily reports do not add up to the monthly one.
//
// Nothing here is wrong with any entry. A timestamp is a point in time and a
// day is a local convention, so an operation near midnight belongs to one date
// in the reporting zone and to another in UTC. Two reports built honestly from
// the same data then disagree, and the difference is exactly the operations
// that sit across the boundary.
//
// The zone has to be supplied. Assuming UTC would produce a report that is
// silent precisely when the caller's own boundary differs from it, which is the
// only case worth reporting.
func Daily(operations []Operation, zone *time.Location) ([]Finding, error) {
	if zone == nil {
		return nil, fmt.Errorf("daily: no reporting time zone given; " +
			"this check cannot assume UTC without hiding the case it exists to find")
	}

	straddling := map[straddle][]Operation{}
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return nil, err
		}
		// An operation with no timestamp has no date in either reading, and
		// formatting its zero time would file it under the year one.
		if operation.At.IsZero() {
			return nil, fmt.Errorf("daily: operation %s carries no timestamp, so it belongs "+
				"to no day in either reading", operation.ID)
		}
		local := operation.At.In(zone).Format(dateLayout)
		utc := operation.At.UTC().Format(dateLayout)
		if local == utc {
			continue
		}
		key := straddle{local: local, utc: utc}
		straddling[key] = append(straddling[key], operation)
	}

	return sortFindings(straddleFindings(straddling, zone)), nil
}

// straddle is one pair of dates an operation belongs to at once.
type straddle struct {
	local string
	utc   string
}

// straddleFindings reports each date pair, one finding per currency.
func straddleFindings(straddling map[straddle][]Operation, zone *time.Location) []Finding {
	var findings []Finding
	for _, key := range sortedStraddles(straddling) {
		operations := straddling[key]

		total := NewSum()
		var evidence []Evidence
		for _, operation := range operations {
			for _, leg := range operation.Legs {
				if !isExternal(leg.Account) {
					continue
				}
				total.Add(leg.Amount.Neg())
				evidence = append(evidence, Evidence{
					LegID:   leg.ID,
					Account: leg.Account,
					Role:    leg.Role,
					Amount:  leg.Amount,
				})
			}
		}

		for _, moved := range total.NonZero() {
			findings = append(findings, straddleFinding(key, operations, moved, zone, evidence))
		}
	}
	return findings
}

// straddleFinding describes one date pair in one currency.
func straddleFinding(key straddle, operations []Operation, moved Amount,
	zone *time.Location, evidence []Evidence) Finding {
	summary := fmt.Sprintf(
		"%d operation(s) fall on %s in %s and on %s in UTC: two honest reports of the same data differ by %s",
		len(operations), key.local, zone, key.utc, moved.Abs())

	arithmetic := fmt.Sprintf("%s in %s = %s in UTC; the operations across the boundary carry %s (%v)",
		key.local, zone, key.utc, moved.Value(), operationIDs(operations))

	return newFinding(ModeDayBoundary, joinOperations(operationIDs(operations)), moved.Currency(),
		summary, arithmetic, evidence)
}

// sortedStraddles orders the date pairs so the output does not depend on map
// iteration.
func sortedStraddles(straddling map[straddle][]Operation) []straddle {
	keys := make([]straddle, 0, len(straddling))
	for key := range straddling {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].local != keys[j].local {
			return keys[i].local < keys[j].local
		}
		return keys[i].utc < keys[j].utc
	})
	return keys
}
