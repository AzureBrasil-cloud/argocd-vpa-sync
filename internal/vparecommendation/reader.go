// Package vparecommendation exposes the controller's current, normalized
// view of every opted-in VerticalPodAutoscaler to the rest of the
// application (chiefly the API layer), without those callers needing to
// know anything about controller-runtime or the Kubernetes API server.
package vparecommendation

import (
	"context"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// VpaRecommendationReader lists every VerticalPodAutoscaler opted in via the
// argocd-vpa-updater.argoproj.io/enabled annotation, normalized. A VPA that
// failed annotation validation is still included (with Valid=false and
// ValidationErrors populated) so the dashboard can explain why it isn't
// eligible, rather than silently omitting it.
type VpaRecommendationReader interface {
	ListOptedIn(ctx context.Context) ([]domain.NormalizedVPA, error)
}
