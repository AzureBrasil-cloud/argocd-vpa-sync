package workloadresources

import (
	"context"
	"fmt"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// FakeReader is a static Reader for tests, keyed by container name.
type FakeReader struct {
	ByContainer map[string]domain.ResourceAmount
	Err         error
}

func (r FakeReader) CurrentValues(_ context.Context, _ domain.WorkloadRef, containerName string) (domain.ResourceAmount, error) {
	if r.Err != nil {
		return domain.ResourceAmount{}, r.Err
	}
	v, ok := r.ByContainer[containerName]
	if !ok {
		return domain.ResourceAmount{}, fmt.Errorf("workloadresources: FakeReader has no value configured for container %q", containerName)
	}
	return v, nil
}
