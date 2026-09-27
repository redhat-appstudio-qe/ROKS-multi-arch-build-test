package cli

import (
	"context"
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/config"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

type recordingRunner struct{ command string }

func (r *recordingRunner) Run(context.Context, config.Request) error {
	r.command = config.CommandRun
	return nil
}

func TestManifestNamespaceOverridesCommandDefault(t *testing.T) {
	request := config.Defaults()
	request.TenantNamespace = config.DefaultGitHubTenantNamespace
	manifest := model.RunManifest{Fixture: model.FixtureIdentity{TenantNamespace: config.DefaultGitLabTenantNamespace}}
	if got := namespaceForManifest(request, manifest); got != config.DefaultGitLabTenantNamespace {
		t.Fatalf("namespace = %q, want %q", got, config.DefaultGitLabTenantNamespace)
	}
}
func (r *recordingRunner) CollectLogs(context.Context, config.Request) error {
	r.command = config.CommandCollectLogs
	return nil
}
func (r *recordingRunner) Cleanup(context.Context, config.Request) error {
	r.command = config.CommandCleanup
	return nil
}

func TestExecuteDispatches(t *testing.T) {
	for _, command := range []string{config.CommandRun, config.CommandCollectLogs, config.CommandCleanup} {
		runner := &recordingRunner{}
		req := config.Defaults()
		req.Command = command
		req.ClusterServer = "https://api.example"
		req.Provider = config.ProviderGitHub
		if command != config.CommandRun {
			req.RunID = "run-1"
		}
		if err := Execute(context.Background(), req, runner); err != nil {
			t.Fatal(err)
		}
		if runner.command != command {
			t.Fatalf("dispatched %q as %q", command, runner.command)
		}
	}
}
