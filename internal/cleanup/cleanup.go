package cleanup

import (
	"context"
	"fmt"
	"strings"

	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

const (
	ManagedByLabel = "app.konflux-ci.org/managed-by"
	RunIDLabel     = "app.konflux.org/run-id"
	ManagedByValue = "konflux-test"
)

type Resource struct {
	Kind      string
	Namespace string
	Name      string
	Labels    map[string]string
}

type ResourceClient interface {
	List(context.Context, string) ([]Resource, error)
	Get(context.Context, Resource) (Resource, error)
	Delete(context.Context, Resource) error
}

type Refusal struct {
	Resource Resource `json:"resource"`
	Reason   string   `json:"reason"`
}

type Result struct {
	Deleted []Resource `json:"deleted"`
	Refused []Refusal  `json:"refused"`
}

type Service struct {
	Store  evidence.ManifestStore
	Client ResourceClient
}

func (s Service) Cleanup(ctx context.Context, runID, targetServer string) (Result, error) {
	if s.Client == nil {
		return Result{}, fmt.Errorf("resource client is required")
	}
	manifest, err := s.Store.Load(runID)
	if err != nil {
		return Result{}, fmt.Errorf("load manifest: %w", err)
	}
	if manifest.TargetClusterServer != targetServer {
		return Result{}, fmt.Errorf("target cluster mismatch: manifest=%q requested=%q", manifest.TargetClusterServer, targetServer)
	}
	resources, err := s.Client.List(ctx, runID)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	for _, resource := range resources {
		if reason := refusalReason(resource, manifest); reason != "" {
			result.Refused = append(result.Refused, Refusal{Resource: resource, Reason: reason})
			continue
		}
		current, err := s.Client.Get(ctx, resource)
		if err != nil {
			result.Refused = append(result.Refused, Refusal{Resource: resource, Reason: "re-read failed: " + err.Error()})
			continue
		}
		if reason := refusalReason(current, manifest); reason != "" {
			result.Refused = append(result.Refused, Refusal{Resource: current, Reason: reason})
			continue
		}
		if err := s.Client.Delete(ctx, current); err != nil {
			return result, fmt.Errorf("delete %s/%s: %w", current.Namespace, current.Name, err)
		}
		result.Deleted = append(result.Deleted, current)
	}
	return result, nil
}

func refusalReason(resource Resource, manifest model.RunManifest) string {
	if (strings.EqualFold(resource.Kind, "Namespace") && resource.Name == manifest.Fixture.TenantNamespace) || (strings.EqualFold(resource.Kind, "Application") && resource.Name == manifest.Fixture.Application) || strings.EqualFold(resource.Kind, "Component") {
		return "persistent fixture resource"
	}
	if strings.EqualFold(resource.Kind, "PipelineRun") {
		return "acceptance PipelineRuns are never deleted"
	}
	if resource.Labels[ManagedByLabel] != ManagedByValue {
		return "missing exact managed-by label"
	}
	if resource.Labels[RunIDLabel] != manifest.RunID {
		return "missing exact run-id label"
	}
	return ""
}
