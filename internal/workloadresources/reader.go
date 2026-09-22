// Package workloadresources reads the resources.requests currently declared
// for one container of the live Kubernetes workload a VPA targets --
// straight from the Deployment/StatefulSet/CronJob object on the API
// server, not from Git. The dashboard needs a "current value" to diff
// against a VPA's recommendation, and the live workload is what that
// recommendation is actually about: a VPA measures the Pods that are really
// running, and a pending (or stuck) Argo CD sync can leave Git declaring
// something the cluster doesn't -- reconciling that gap is this project's
// whole job, not a precondition for showing a useful number. Write-back
// still has to read and patch the Git-declared value (see
// internal/gitrepo, internal/patcher); this package is read-only and never
// touches Git.
package workloadresources

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// ErrUnsupportedKind is returned for a domain.WorkloadRef.Kind other than
// the ones the VpaGitOpsBinding CRD's targetRef schema allows (Deployment,
// StatefulSet, CronJob).
var ErrUnsupportedKind = errors.New("workloadresources: unsupported workload kind")

// ErrContainerNotFound is returned when the live workload's pod template
// has no container with the requested name.
var ErrContainerNotFound = errors.New("workloadresources: container not found in live workload")

// Reader reads the current resources.requests for one container of a live
// workload.
type Reader interface {
	CurrentValues(ctx context.Context, workload domain.WorkloadRef, containerName string) (domain.ResourceAmount, error)
}

// ClusterReader is the default Reader, backed by a Kubernetes client.
type ClusterReader struct {
	Client client.Client
}

// NewClusterReader builds a ClusterReader.
func NewClusterReader(c client.Client) *ClusterReader {
	return &ClusterReader{Client: c}
}

// CurrentValues implements Reader.
func (r *ClusterReader) CurrentValues(ctx context.Context, workload domain.WorkloadRef, containerName string) (domain.ResourceAmount, error) {
	containers, err := r.podSpecContainers(ctx, workload)
	if err != nil {
		return domain.ResourceAmount{}, err
	}
	for _, c := range containers {
		if c.Name == containerName {
			return toResourceAmount(c.Resources.Requests), nil
		}
	}
	return domain.ResourceAmount{}, fmt.Errorf("%w: %s/%s container %q", ErrContainerNotFound, workload.Namespace, workload.Name, containerName)
}

func (r *ClusterReader) podSpecContainers(ctx context.Context, workload domain.WorkloadRef) ([]corev1.Container, error) {
	key := client.ObjectKey{Namespace: workload.Namespace, Name: workload.Name}

	switch workload.Kind {
	case "Deployment":
		var obj appsv1.Deployment
		if err := r.Client.Get(ctx, key, &obj); err != nil {
			return nil, fmt.Errorf("workloadresources: get Deployment %s: %w", key, err)
		}
		return obj.Spec.Template.Spec.Containers, nil
	case "StatefulSet":
		var obj appsv1.StatefulSet
		if err := r.Client.Get(ctx, key, &obj); err != nil {
			return nil, fmt.Errorf("workloadresources: get StatefulSet %s: %w", key, err)
		}
		return obj.Spec.Template.Spec.Containers, nil
	case "CronJob":
		var obj batchv1.CronJob
		if err := r.Client.Get(ctx, key, &obj); err != nil {
			return nil, fmt.Errorf("workloadresources: get CronJob %s: %w", key, err)
		}
		return obj.Spec.JobTemplate.Spec.Template.Spec.Containers, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedKind, workload.Kind)
	}
}

func toResourceAmount(rl corev1.ResourceList) domain.ResourceAmount {
	var out domain.ResourceAmount
	if cpu, ok := rl[corev1.ResourceCPU]; ok {
		c := cpu.DeepCopy()
		out.CPU = &c
	}
	if mem, ok := rl[corev1.ResourceMemory]; ok {
		m := mem.DeepCopy()
		out.Memory = &m
	}
	return out
}
