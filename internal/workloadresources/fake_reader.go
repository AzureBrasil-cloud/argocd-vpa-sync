package workloadresources

import (
	"context"
	"fmt"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// FakeReader is a static Reader for tests, keyed by container name.
// ByContainer holds each container's requests; LimitsByContainer (optional)
// its limits.
type FakeReader struct {
	ByContainer       map[string]domain.ResourceAmount
	LimitsByContainer map[string]domain.ResourceAmount
	Err               error
}

func (r FakeReader) CurrentValues(_ context.Context, _ domain.WorkloadRef, containerName string) (Values, error) {
	if r.Err != nil {
		return Values{}, r.Err
	}
	v, ok := r.ByContainer[containerName]
	if !ok {
		return Values{}, fmt.Errorf("workloadresources: FakeReader has no value configured for container %q", containerName)
	}
	return Values{Requests: v, Limits: r.LimitsByContainer[containerName]}, nil
}
