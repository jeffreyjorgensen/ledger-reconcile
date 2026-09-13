package reconcile

import (
	"fmt"
	"sort"
)

// Stable orders and stable names.
//
// Findings are compared by content and read by people, so nothing about them
// may depend on the order the caller happened to pass things in, or on Go's
// map iteration. Every list that reaches a finding comes through here.

func sortedRecords(records []ExternalRecord) []ExternalRecord {
	ordered := make([]ExternalRecord, len(records))
	copy(ordered, records)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	return ordered
}

// operationIDs lists the ids of operations in a stable order.
func operationIDs(operations []Operation) []string {
	ids := make([]string, 0, len(operations))
	for _, operation := range operations {
		ids = append(ids, operation.ID)
	}
	sort.Strings(ids)
	return ids
}

// recordIDs lists record ids in a stable order.
func recordIDs(records []ExternalRecord) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	sort.Strings(ids)
	return ids
}

// joinOperations names the operations a finding is about, in the order given.
func joinOperations(operations []string) string {
	switch len(operations) {
	case 0:
		return ""
	case 1:
		return operations[0]
	default:
		return operations[0] + fmt.Sprintf(" (+%d more)", len(operations)-1)
	}
}
