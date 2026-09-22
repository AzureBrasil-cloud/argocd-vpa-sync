package statestore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// stateDataKey is the Secret data key holding the JSON-encoded StateDocument.
const stateDataKey = "state.json"

// Size thresholds relative to the ~1MiB Kubernetes Secret limit: warn well
// before it, and refuse to write (ErrNearCapacity) with enough headroom that
// the failure is this package's own clean error, never an opaque rejection
// from the API server.
const (
	warnSizeBytes = 800 * 1024
	maxSizeBytes  = 950 * 1024
)

// SecretStore is the default StateStore: a single Kubernetes Secret holding
// one JSON document, updated with optimistic concurrency on the Secret's
// resourceVersion.
type SecretStore struct {
	Client          client.Client
	Namespace       string
	Name            string
	RetentionPolicy RetentionPolicy

	// Now is overridable for deterministic tests; defaults to time.Now.
	Now func() time.Time

	// OnNearCapacity, if set, is called (in addition to returning
	// ErrNearCapacity) once the compacted document exceeds warnSizeBytes,
	// letting the caller surface a health/readiness condition.
	OnNearCapacity func(sizeBytes int)
}

// NewSecretStore builds a SecretStore backed by the named Secret.
func NewSecretStore(c client.Client, namespace, name string) *SecretStore {
	return &SecretStore{
		Client:          c,
		Namespace:       namespace,
		Name:            name,
		RetentionPolicy: DefaultRetentionPolicy(),
		Now:             time.Now,
	}
}

func (s *SecretStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *SecretStore) Get(ctx context.Context) (*domain.StateDocument, string, error) {
	var secret corev1.Secret
	err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Namespace, Name: s.Name}, &secret)
	if apierrors.IsNotFound(err) {
		return domain.NewEmptyStateDocument(), "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("statestore: get secret %s/%s: %w", s.Namespace, s.Name, err)
	}
	doc, err := decode(secret.Data[stateDataKey])
	if err != nil {
		return nil, "", fmt.Errorf("statestore: decode state document in secret %s/%s: %w", s.Namespace, s.Name, err)
	}
	return doc, secret.ResourceVersion, nil
}

func (s *SecretStore) Update(ctx context.Context, mutate func(*domain.StateDocument) error) error {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var secret corev1.Secret
		getErr := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Namespace, Name: s.Name}, &secret)
		notFound := apierrors.IsNotFound(getErr)
		if getErr != nil && !notFound {
			return fmt.Errorf("statestore: get secret %s/%s: %w", s.Namespace, s.Name, getErr)
		}

		var doc *domain.StateDocument
		if notFound {
			doc = domain.NewEmptyStateDocument()
		} else {
			var decodeErr error
			doc, decodeErr = decode(secret.Data[stateDataKey])
			if decodeErr != nil {
				return fmt.Errorf("statestore: decode state document in secret %s/%s: %w", s.Namespace, s.Name, decodeErr)
			}
		}

		if err := mutate(doc); err != nil {
			return err
		}

		Compact(doc, s.RetentionPolicy, s.now())

		data, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("statestore: marshal state document: %w", err)
		}
		if len(data) >= warnSizeBytes && s.OnNearCapacity != nil {
			s.OnNearCapacity(len(data))
		}
		if len(data) > maxSizeBytes {
			return fmt.Errorf("%w: %d bytes after compaction (limit %d)", ErrNearCapacity, len(data), maxSizeBytes)
		}

		if notFound {
			newSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: s.Name, Namespace: s.Namespace},
				Data:       map[string][]byte{stateDataKey: data},
			}
			if err := s.Client.Create(ctx, newSecret); err != nil {
				return fmt.Errorf("statestore: create secret %s/%s: %w", s.Namespace, s.Name, err)
			}
			return nil
		}

		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[stateDataKey] = data
		return s.Client.Update(ctx, &secret)
	})

	if retryErr != nil && apierrors.IsConflict(retryErr) {
		return fmt.Errorf("%w: %v", ErrOptimisticLockConflict, retryErr)
	}
	return retryErr
}

func decode(data []byte) (*domain.StateDocument, error) {
	if len(data) == 0 {
		return domain.NewEmptyStateDocument(), nil
	}
	var doc domain.StateDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Operations == nil {
		doc.Operations = map[string]domain.OperationState{}
	}
	if doc.PendingSelections == nil {
		doc.PendingSelections = []domain.PendingSelection{}
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = domain.CurrentStateSchemaVersion
	}
	return &doc, nil
}
