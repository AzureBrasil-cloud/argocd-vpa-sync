package statestore

import (
	"testing"
	"time"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func TestCompact_ExpiresOrphanedPendingSelectionsByTTL(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	doc := &domain.StateDocument{
		Operations: map[string]domain.OperationState{},
		PendingSelections: []domain.PendingSelection{
			{IdempotencyKey: "old", SelectedAt: now.Add(-48 * time.Hour)},
			{IdempotencyKey: "recent", SelectedAt: now.Add(-1 * time.Hour)},
		},
	}

	Compact(doc, RetentionPolicy{PendingSelectionTTL: 24 * time.Hour}, now)

	if len(doc.PendingSelections) != 1 || doc.PendingSelections[0].IdempotencyKey != "recent" {
		t.Fatalf("expected only the recent selection to survive, got %+v", doc.PendingSelections)
	}
}

func TestCompact_KeepsPendingSelectionWithMatchingOperationRegardlessOfAge(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	doc := &domain.StateDocument{
		Operations: map[string]domain.OperationState{
			"key-1": {IdempotencyKey: "key-1", Status: domain.OperationApplying, UpdatedAt: now.Add(-48 * time.Hour)},
		},
		PendingSelections: []domain.PendingSelection{
			{IdempotencyKey: "key-1", SelectedAt: now.Add(-48 * time.Hour)},
		},
	}

	Compact(doc, RetentionPolicy{PendingSelectionTTL: 24 * time.Hour}, now)

	if len(doc.PendingSelections) != 1 {
		t.Fatalf("expected the selection backed by an operation to survive, got %+v", doc.PendingSelections)
	}
}

func TestCompact_CapsTerminalOperationsPerKey(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	doc := &domain.StateDocument{Operations: map[string]domain.OperationState{}}

	for i := 0; i < 8; i++ {
		key := opKey(i)
		doc.Operations[key] = domain.OperationState{
			IdempotencyKey: key,
			VPANamespace:   "payments",
			VPAName:        "checkout-api-vpa",
			ContainerName:  "app",
			Status:         domain.OperationApplied,
			UpdatedAt:      now.Add(-time.Duration(i) * time.Hour), // i=0 is most recent
		}
	}

	Compact(doc, RetentionPolicy{MaxOperationsPerKey: 3}, now)

	if len(doc.Operations) != 3 {
		t.Fatalf("expected exactly 3 operations to remain, got %d", len(doc.Operations))
	}
	for i := 0; i < 3; i++ {
		if _, ok := doc.Operations[opKey(i)]; !ok {
			t.Fatalf("expected the %d most recent operations to survive, missing %s", 3, opKey(i))
		}
	}
	for i := 3; i < 8; i++ {
		if _, ok := doc.Operations[opKey(i)]; ok {
			t.Fatalf("expected older operation %s to be pruned", opKey(i))
		}
	}
}

func TestCompact_DoesNotCapNonTerminalOperations(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	doc := &domain.StateDocument{Operations: map[string]domain.OperationState{}}

	for i := 0; i < 8; i++ {
		key := opKey(i)
		doc.Operations[key] = domain.OperationState{
			IdempotencyKey: key,
			VPANamespace:   "payments",
			VPAName:        "checkout-api-vpa",
			ContainerName:  "app",
			Status:         domain.OperationPending,
			UpdatedAt:      now.Add(-time.Duration(i) * time.Hour),
		}
	}

	Compact(doc, RetentionPolicy{MaxOperationsPerKey: 3}, now)

	if len(doc.Operations) != 8 {
		t.Fatalf("expected non-terminal operations to be exempt from the per-key cap, got %d remaining", len(doc.Operations))
	}
}

func TestCompact_AgesOutOldOperationsRegardlessOfStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	doc := &domain.StateDocument{
		Operations: map[string]domain.OperationState{
			"ancient": {IdempotencyKey: "ancient", Status: domain.OperationPending, UpdatedAt: now.Add(-100 * 24 * time.Hour)},
			"recent":  {IdempotencyKey: "recent", Status: domain.OperationPending, UpdatedAt: now.Add(-1 * time.Hour)},
		},
	}

	Compact(doc, RetentionPolicy{MaxOperationAge: 90 * 24 * time.Hour}, now)

	if _, ok := doc.Operations["ancient"]; ok {
		t.Fatalf("expected the ancient operation to be aged out")
	}
	if _, ok := doc.Operations["recent"]; !ok {
		t.Fatalf("expected the recent operation to survive")
	}
}

func TestCompact_TruncatesErrorMessage(t *testing.T) {
	now := time.Now()
	longMsg := make([]byte, 1000)
	for i := range longMsg {
		longMsg[i] = 'x'
	}
	doc := &domain.StateDocument{
		Operations: map[string]domain.OperationState{
			"key-1": {IdempotencyKey: "key-1", ErrorMessage: string(longMsg), UpdatedAt: now},
		},
	}

	Compact(doc, RetentionPolicy{MaxErrorMessageBytes: 100}, now)

	if len(doc.Operations["key-1"].ErrorMessage) != 100 {
		t.Fatalf("expected error message to be truncated to 100 bytes, got %d", len(doc.Operations["key-1"].ErrorMessage))
	}
}

func TestCompact_SetsUpdatedAt(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	doc := &domain.StateDocument{Operations: map[string]domain.OperationState{}}
	Compact(doc, DefaultRetentionPolicy(), now)
	if !doc.UpdatedAt.Equal(now) {
		t.Fatalf("expected UpdatedAt to be set to now, got %v", doc.UpdatedAt)
	}
}

func opKey(i int) string {
	return "key-" + string(rune('a'+i))
}
