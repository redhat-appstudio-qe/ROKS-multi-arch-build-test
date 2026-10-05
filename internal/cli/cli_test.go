package cli

import (
	"context"
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/config"
)

type recordingRunner struct{ command string }

func (r *recordingRunner) Run(context.Context, config.Request) error {
	r.command = config.CommandRun
	return nil
}

func TestExecuteDispatchesRun(t *testing.T) {
	runner := &recordingRunner{}
	req := config.Defaults()
	req.Command = config.CommandRun
	req.ClusterServer = "https://api.example"
	req.Provider = config.ProviderGitHub
	if err := Execute(context.Background(), req, runner); err != nil {
		t.Fatal(err)
	}
	if runner.command != config.CommandRun {
		t.Fatalf("dispatched %q as %q", config.CommandRun, runner.command)
	}
}
