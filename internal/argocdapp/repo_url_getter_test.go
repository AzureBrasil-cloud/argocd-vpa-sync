package argocdapp

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRepoURLGetter_SingleSource(t *testing.T) {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(applicationGVK)
	app.SetName("payments-api")
	app.SetNamespace("argocd")
	_ = unstructured.SetNestedField(app.Object, "https://git.example.com/team/app.git", "spec", "source", "repoURL")

	c := fake.NewClientBuilder().WithObjects(app).Build()
	g := NewRepoURLGetter(c)

	urls, err := g.GetRepoURLs(context.Background(), "argocd", "payments-api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://git.example.com/team/app.git" {
		t.Fatalf("unexpected urls: %v", urls)
	}
}

func TestRepoURLGetter_MultiSource(t *testing.T) {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(applicationGVK)
	app.SetName("payments-api")
	app.SetNamespace("argocd")
	sources := []interface{}{
		map[string]interface{}{"repoURL": "https://git.example.com/team/app.git"},
		map[string]interface{}{"repoURL": "https://git.example.com/team/charts.git"},
	}
	_ = unstructured.SetNestedSlice(app.Object, sources, "spec", "sources")

	c := fake.NewClientBuilder().WithObjects(app).Build()
	g := NewRepoURLGetter(c)

	urls, err := g.GetRepoURLs(context.Background(), "argocd", "payments-api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 urls, got %v", urls)
	}
}

func TestRepoURLGetter_NotFound(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	g := NewRepoURLGetter(c)
	if _, err := g.GetRepoURLs(context.Background(), "argocd", "missing"); err == nil {
		t.Fatalf("expected an error for a missing Application")
	}
}
