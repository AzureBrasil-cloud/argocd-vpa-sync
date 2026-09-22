// Package argocdapp reads the Argo CD Application CR purely to cross-check
// a VPA's declared repo-url annotation, using unstructured access so this
// controller does not need to vendor the full Argo CD API types for one
// read-only field lookup.
package argocdapp

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var applicationGVK = schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"}

// RepoURLGetter implements resolver.ArgoCDApplicationRepoURLGetter.
type RepoURLGetter struct {
	Client client.Client
}

// NewRepoURLGetter builds a RepoURLGetter. c only needs read (get) access to
// argoproj.io Applications.
func NewRepoURLGetter(c client.Client) *RepoURLGetter {
	return &RepoURLGetter{Client: c}
}

// GetRepoURLs returns every repoURL declared by the named Application's
// spec.source and/or spec.sources (multi-source Applications).
func (g *RepoURLGetter) GetRepoURLs(ctx context.Context, namespace, name string) ([]string, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(applicationGVK)
	if err := g.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, fmt.Errorf("argocdapp: get Application %s/%s: %w", namespace, name, err)
	}

	var urls []string
	if repoURL, found, _ := unstructured.NestedString(obj.Object, "spec", "source", "repoURL"); found && repoURL != "" {
		urls = append(urls, repoURL)
	}
	if sources, found, _ := unstructured.NestedSlice(obj.Object, "spec", "sources"); found {
		for _, s := range sources {
			m, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			if u, ok := m["repoURL"].(string); ok && u != "" {
				urls = append(urls, u)
			}
		}
	}
	return urls, nil
}
