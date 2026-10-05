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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type LiveRunner struct {
	Prompt workflow.Confirmation
}

func NewLiveRunner() Runner { return LiveRunner{Prompt: NewInteractiveConfirmationPrompt()} }

func (runner LiveRunner) Run(ctx context.Context, request config.Request) error {
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
	stage.Prompt = runner.Prompt
	store := evidence.NewManifestStore(request.StateDir)
	manifest, err := (workflow.Runner{Store: store, Stage: stage}).Run(ctx, workflow.Options{
		RunID:            request.RunID,
		Provider:         request.Provider,
		ClusterServer:    request.ClusterServer,
		Resume:           request.ResumeRunID != "",
		FixtureNamespace: request.TenantNamespace,
		Application:      request.ApplicationName,
		Timeouts: workflow.PhaseTimeouts{
			Preflight: request.Timeouts.Preflight,
			Build:     request.Timeouts.Build,
		},
	})
	if err != nil {
		finalizeErr := (Lifecycle{Store: store, Namespace: stage.Namespace, Collector: collector.Collector{StateDir: request.StateDir}, Prompt: stage.Prompt, Sources: collector.KubernetesSources(clients.Dynamic, manifest.Fixture.TenantNamespace, manifest.RunID)}).Finalize(ctx, manifest, err)
		if finalizeErr != nil {
			return fmt.Errorf("run %s failed at %s: %w; finalization failed: %v", manifest.RunID, manifest.Phase, err, finalizeErr)
		}
		return fmt.Errorf("run %s failed at %s: %w", manifest.RunID, manifest.Phase, err)
	}
	if err := (Lifecycle{Store: store, Namespace: stage.Namespace, Prompt: stage.Prompt}).Finalize(ctx, manifest, nil); err != nil {
		return fmt.Errorf("run %s completed but finalization failed: %w", manifest.RunID, err)
	}
	fmt.Printf("completed run %s at phase %s\n", manifest.RunID, manifest.Phase)
	return nil
}

type Lifecycle struct {
	Store     evidence.ManifestStore
	Namespace cleanup.NamespaceService
	Collector collector.Collector
	Prompt    workflow.Confirmation
	Sources   []collector.Source
}

func (l Lifecycle) Finalize(ctx context.Context, manifest model.RunManifest, runErr error) error {
	namespace := manifest.Fixture.TenantNamespace
	if namespace == "" {
		return nil
	}
	info, err := l.Namespace.Inspect(ctx, namespace)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Name != namespace || info.Labels[cleanup.ManagedByLabel] != cleanup.ManagedByValue || info.RunID != manifest.RunID {
		if runErr != nil && manifest.Phase == model.PhaseFailed {
			return nil
		}
		return fmt.Errorf("tenant namespace %s is not owned by run %s", namespace, manifest.RunID)
	}
	if runErr != nil {
		sources := l.Sources
		if sources == nil && l.Namespace.Dynamic != nil {
			sources = collector.KubernetesSources(l.Namespace.Dynamic, namespace, manifest.RunID)
		}
		report, collectErr := l.Collector.CollectAndVerify(ctx, manifest.RunID, manifest, sources)
		manifest.FailureArtifacts = &report
		if l.Store.Root != "" {
			if saveErr := l.Store.Save(manifest); saveErr != nil {
				return fmt.Errorf("save failure artifact report: %w", saveErr)
			}
		}
		if collectErr != nil {
			return collectErr
		}
	}
	if l.Prompt == nil {
		return nil
	}
	approved, err := l.Prompt.Confirm(ctx, fmt.Sprintf("Delete owned tenant namespace %s from run %s", namespace, manifest.RunID))
	if err != nil {
		return err
	}
	if !approved {
		fmt.Printf("retained tenant namespace %s\n", namespace)
		return nil
	}
	if err := l.Namespace.Delete(ctx, namespace, manifest.RunID); err != nil {
		return err
	}
	return l.Namespace.WaitDeleted(ctx, namespace)
}

func providerFor(request config.Request) (providers.Provider, error) {
	switch request.Provider {
	case config.ProviderGitHub:
		return github.NewFromCredentials(request.Credentials.GitHubToken)
	case config.ProviderGitLab:
		return gitlab.NewFromCredentials(request.Credentials.GitLabToken, request.Credentials.GitLabAPIURL)
	default:
		return nil, fmt.Errorf("unsupported provider %q", request.Provider)
	}
}
