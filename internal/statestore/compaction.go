package statestore

import (
	"sort"
	"time"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// RetentionPolicy bounds how much history StateDocument may accumulate.
// Compact enforces it on every write so the backing Secret never grows
// without limit.
type RetentionPolicy struct {
	// PendingSelectionTTL: drop a pending selection older than this if it has
	// no matching entry in Operations (an abandoned selection the user never
	// confirmed, or whose operation was itself pruned).
	PendingSelectionTTL time.Duration

	// MaxOperationsPerKey caps how many *terminal* (applied/failed)
	// operations are kept per (vpaNamespace, vpaName, containerName); the
	// oldest (by UpdatedAt) beyond this count are dropped. Non-terminal
	// operations (pending/applying/conflict) are never pruned by this rule --
	// they represent work still in flight.
	MaxOperationsPerKey int

	// MaxOperationAge ages out any operation entry, terminal or not, older
	// than this.
	MaxOperationAge time.Duration

	// MaxErrorMessageBytes truncates OperationState.ErrorMessage before
	// storage.
	MaxErrorMessageBytes int
}

// DefaultRetentionPolicy is used by NewSecretStore.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		PendingSelectionTTL:  24 * time.Hour,
		MaxOperationsPerKey:  5,
		MaxOperationAge:      90 * 24 * time.Hour,
		MaxErrorMessageBytes: 500,
	}
}

// Compact prunes doc in place according to policy, as evaluated at `now`.
func Compact(doc *domain.StateDocument, policy RetentionPolicy, now time.Time) {
	if doc.Operations == nil {
		doc.Operations = map[string]domain.OperationState{}
	}

	if policy.MaxErrorMessageBytes > 0 {
		for k, op := range doc.Operations {
			if len(op.ErrorMessage) > policy.MaxErrorMessageBytes {
				op.ErrorMessage = op.ErrorMessage[:policy.MaxErrorMessageBytes]
				doc.Operations[k] = op
			}
		}
	}

	if policy.MaxOperationAge > 0 {
		for k, op := range doc.Operations {
			if now.Sub(op.UpdatedAt) > policy.MaxOperationAge {
				delete(doc.Operations, k)
			}
		}
	}

	if policy.MaxOperationsPerKey > 0 {
		capTerminalOperationsPerKey(doc, policy.MaxOperationsPerKey)
	}

	kept := make([]domain.PendingSelection, 0, len(doc.PendingSelections))
	for _, sel := range doc.PendingSelections {
		_, hasOperation := doc.Operations[sel.IdempotencyKey]
		if !hasOperation && policy.PendingSelectionTTL > 0 && now.Sub(sel.SelectedAt) > policy.PendingSelectionTTL {
			continue
		}
		kept = append(kept, sel)
	}
	doc.PendingSelections = kept

	doc.UpdatedAt = now
}

func isTerminal(status domain.OperationStatus) bool {
	return status == domain.OperationApplied || status == domain.OperationFailed
}

func capTerminalOperationsPerKey(doc *domain.StateDocument, max int) {
	type entry struct {
		key string
		op  domain.OperationState
	}

	groups := map[string][]entry{}
	for k, op := range doc.Operations {
		if !isTerminal(op.Status) {
			continue
		}
		groupKey := op.VPANamespace + "/" + op.VPAName + "/" + op.ContainerName
		groups[groupKey] = append(groups[groupKey], entry{key: k, op: op})
	}

	for _, entries := range groups {
		if len(entries) <= max {
			continue
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].op.UpdatedAt.After(entries[j].op.UpdatedAt)
		})
		for _, e := range entries[max:] {
			delete(doc.Operations, e.key)
		}
	}
}
