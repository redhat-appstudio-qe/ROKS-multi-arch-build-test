package cli

import (
	"context"
	"fmt"

	"github.com/redhat-appstudio/konflux-test/internal/cleanup"
	"github.com/redhat-appstudio/konflux-test/internal/cluster"
	"github.com/redhat-appstudio/konflux-test/internal/collector"
	"github.com/redhat-appstudio/konflux-test/internal/config"
	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
	"github.com/redhat-appstudio/konflux-test/internal/providers/github"
	"github.com/redhat-appstudio/konflux-test/internal/providers/gitlab"
	"github.com/redhat-appstudio/konflux-test/internal/workflow"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type LiveRunner struct{}

func NewLiveRunner() Runner { return LiveRunner{} }

func (LiveRunner) Run(ctx context.Context, request config.Request) error {
	clients, err := cluster.NewClientSet("")
	if err != nil {
		return err
	}
	provider, err := providerFor(request)
	if err != nil {
		return err
	}
	stage, err := workflow.NewLiveStage(request, clients, provider)
	if err != nil {
		return err
	}
	runID := request.RunID
	manifest, err := (workflow.Runner{Store: evidence.NewManifestStore(request.StateDir), Stage: stage}).Run(ctx, workflow.Options{
		RunID:            runID,
		Provider:         request.Provider,
		ClusterServer:    request.ClusterServer,
		Resume:           request.ResumeRunID != "",
		FixtureNamespace: request.TenantNamespace,
		Application:      request.ApplicationName,
		Timeouts: workflow.PhaseTimeouts{
			Preflight: request.Timeouts.Preflight,
			Build:     request.Timeouts.Build,
			Pruning:   request.Timeouts.Pruning,
			Archive:   request.Timeouts.Archive,
		},
	})
	if err != nil {
		return fmt.Errorf("run %s failed at %s: %w", manifest.RunID, manifest.Phase, err)
	}
	fmt.Printf("completed run %s at phase %s\n", manifest.RunID, manifest.Phase)
	return nil
}

func (LiveRunner) CollectLogs(ctx context.Context, request config.Request) error {
	ctx, cancel := context.WithTimeout(ctx, request.Timeouts.Collect)
	defer cancel()
	store := evidence.NewManifestStore(request.StateDir)
	manifest, err := store.Load(request.RunID)
	if err != nil {
		return err
	}
	clients, err := cluster.NewClientSet("")
	if err != nil {
		return err
	}
	namespace := namespaceForManifest(request, manifest)
	report, err := (collector.Collector{StateDir: request.StateDir}).Collect(ctx, request.RunID, manifest, append([]collector.Source{{Name: "manifest", Collect: func(context.Context, string) error { return nil }}}, collector.KubernetesSources(clients.Dynamic, namespace, request.RunID)...))
	if err != nil {
		return err
	}
	fmt.Printf("collection report for %s: %d sources\n", report.RunID, len(report.Attempts))
	return nil
}

func namespaceForManifest(request config.Request, manifest model.RunManifest) string {
	if manifest.Fixture.TenantNamespace != "" {
		return manifest.Fixture.TenantNamespace
	}
	return request.TenantNamespace
}

func (LiveRunner) Cleanup(ctx context.Context, request config.Request) error {
	store := evidence.NewManifestStore(request.StateDir)
	clients, err := cluster.NewClientSet("")
	if err != nil {
		return err
	}
	result, err := (cleanup.Service{Store: store, Client: dynamicResourceClient{client: clients.Dynamic}}).Cleanup(ctx, request.RunID, request.ClusterServer)
	if err != nil {
		return err
	}
	fmt.Printf("cleanup deleted %d resources and refused %d\n", len(result.Deleted), len(result.Refused))
	return nil
}

func providerFor(request config.Request) (providers.Provider, error) {
	switch request.Provider {
	case config.ProviderGitHub:
		return github.NewFromCredentials(request.Credentials.GitHubToken, request.Credentials.GitHubOrg)
	case config.ProviderGitLab:
		return gitlab.NewFromCredentials(request.Credentials.GitLabToken, request.Credentials.GitLabAPIURL, request.Credentials.GitLabGroupID)
	default:
		return nil, fmt.Errorf("unsupported provider %q", request.Provider)
	}
}

type dynamicResourceClient struct{ client dynamic.Interface }

var cleanupGVRs = []schema.GroupVersionResource{
	{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"},
	{Group: "tekton.dev", Version: "v1", Resource: "taskruns"},
}

func (c dynamicResourceClient) List(ctx context.Context, runID string) ([]cleanup.Resource, error) {
	var resources []cleanup.Resource
	selector := cleanup.RunIDLabel + "=" + runID
	for _, gvr := range cleanupGVRs {
		list, err := c.client.Resource(gvr).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return nil, err
		}
		for _, object := range list.Items {
			resources = append(resources, cleanup.Resource{Kind: object.GetKind(), Namespace: object.GetNamespace(), Name: object.GetName(), Labels: object.GetLabels()})
		}
	}
	return resources, nil
}

func (c dynamicResourceClient) Get(ctx context.Context, resource cleanup.Resource) (cleanup.Resource, error) {
	object, err := c.object(ctx, resource, metav1.GetOptions{})
	if err != nil {
		return cleanup.Resource{}, err
	}
	return cleanup.Resource{Kind: object.GetKind(), Namespace: object.GetNamespace(), Name: object.GetName(), Labels: object.GetLabels()}, nil
}

func (c dynamicResourceClient) Delete(ctx context.Context, resource cleanup.Resource) error {
	_, err := c.object(ctx, resource, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for _, gvr := range cleanupGVRs {
		if gvr.Resource == "pipelineruns" && resource.Kind == "PipelineRun" || gvr.Resource == "taskruns" && resource.Kind == "TaskRun" {
			return c.client.Resource(gvr).Namespace(resource.Namespace).Delete(ctx, resource.Name, metav1.DeleteOptions{})
		}
	}
	return fmt.Errorf("unsupported cleanup resource %s/%s", resource.Kind, resource.Name)
}

func (c dynamicResourceClient) object(ctx context.Context, resource cleanup.Resource, options metav1.GetOptions) (*unstructured.Unstructured, error) {
	for _, gvr := range cleanupGVRs {
		if gvr.Resource == "pipelineruns" && resource.Kind == "PipelineRun" || gvr.Resource == "taskruns" && resource.Kind == "TaskRun" {
			return c.client.Resource(gvr).Namespace(resource.Namespace).Get(ctx, resource.Name, options)
		}
	}
	return nil, fmt.Errorf("unsupported cleanup resource %s/%s", resource.Kind, resource.Name)
}
