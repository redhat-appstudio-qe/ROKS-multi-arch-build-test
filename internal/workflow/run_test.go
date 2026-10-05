package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

type fakeStage struct {
	calls     []string
	deadlines map[string]bool
	fail      string
}

func (s *fakeStage) call(ctx context.Context, name string) error {
	s.calls = append(s.calls, name)
	if _, ok := ctx.Deadline(); ok {
		if s.deadlines == nil {
			s.deadlines = map[string]bool{}
		}
		s.deadlines[name] = true
	}
	if s.fail == name {
		return context.Canceled
	}
	return nil
}
func (s *fakeStage) Preflight(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "preflight")
}
func (s *fakeStage) EnsureFixture(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "fixture")
}
func (s *fakeStage) TriggerComponents(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "trigger")
}
func (s *fakeStage) VerifyBuilds(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "builds")
}
func (s *fakeStage) VerifyBuildOutputs(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "build-outputs")
}

func TestRunnerPersistsSuccessfulPhaseSequence(t *testing.T) {
	stage := &fakeStage{}
	runner := Runner{Store: evidence.NewManifestStore(t.TempDir()), Stage: stage, Now: func() time.Time { return time.Unix(100, 0) }}
	manifest, err := runner.Run(context.Background(), Options{RunID: "run-1", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant", Application: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Phase != model.PhaseCompleted || len(stage.calls) != 5 {
		t.Fatalf("phase=%q calls=%v", manifest.Phase, stage.calls)
	}
	want := []string{"preflight", "fixture", "trigger", "builds", "build-outputs"}
	for index, call := range want {
		if stage.calls[index] != call {
			t.Fatalf("calls=%v, want %v", stage.calls, want)
		}
	}
}

func TestRunnerPersistsFailure(t *testing.T) {
	stage := &fakeStage{fail: "build-outputs"}
	runner := Runner{Store: evidence.NewManifestStore(t.TempDir()), Stage: stage}
	manifest, err := runner.Run(context.Background(), Options{RunID: "run-2", Provider: "github", ClusterServer: "https://api.example"})
	if err == nil || manifest.Phase != model.PhaseFailed || manifest.Failure == nil {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
}

func TestRunnerAppliesPhaseTimeouts(t *testing.T) {
	stage := &fakeStage{}
	runner := Runner{Store: evidence.NewManifestStore(t.TempDir()), Stage: stage}
	_, err := runner.Run(context.Background(), Options{
		RunID:         "run-timeout",
		Provider:      "github",
		ClusterServer: "https://api.example",
		Timeouts:      PhaseTimeouts{Preflight: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stage.deadlines["preflight"] {
		t.Fatalf("preflight context had no deadline: %#v", stage.deadlines)
	}
}

func TestRunnerRejectsResumeFromFailedRun(t *testing.T) {
	store := evidence.NewManifestStore(t.TempDir())
	failedStage := &fakeStage{fail: "build-outputs"}
	runner := Runner{Store: store, Stage: failedStage}
	_, _ = runner.Run(context.Background(), Options{RunID: "run-failed", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant"})

	resumed, err := (Runner{Store: store, Stage: &fakeStage{}}).Run(context.Background(), Options{
		RunID:            "run-failed",
		Provider:         "github",
		ClusterServer:    "https://api.example",
		FixtureNamespace: "tenant",
		Resume:           true,
	})
	if err == nil || !strings.Contains(err.Error(), "failed run") || resumed.Phase != model.PhaseFailed {
		t.Fatalf("manifest=%#v err=%v, want failed resume rejection", resumed, err)
	}
}

func TestRunnerRejectsResumeWithDifferentTenant(t *testing.T) {
	store := evidence.NewManifestStore(t.TempDir())
	runner := Runner{Store: store, Stage: &fakeStage{}}
	_, err := runner.Run(context.Background(), Options{RunID: "run-tenant", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}

	resumed, err := (Runner{Store: store, Stage: &fakeStage{}}).Run(context.Background(), Options{
		RunID:            "run-tenant",
		Provider:         "github",
		ClusterServer:    "https://api.example",
		FixtureNamespace: "tenant-b",
		Resume:           true,
	})
	if err == nil || !strings.Contains(err.Error(), "resume ownership mismatch") || resumed.Fixture.TenantNamespace != "tenant-a" {
		t.Fatalf("manifest=%#v err=%v, want tenant ownership rejection", resumed, err)
	}
}

func TestRunnerRejectsResumeWithoutTenant(t *testing.T) {
	store := evidence.NewManifestStore(t.TempDir())
	runner := Runner{Store: store, Stage: &fakeStage{}}
	_, err := runner.Run(context.Background(), Options{RunID: "run-tenant-empty", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}

	resumed, err := (Runner{Store: store, Stage: &fakeStage{}}).Run(context.Background(), Options{
		RunID:         "run-tenant-empty",
		Provider:      "github",
		ClusterServer: "https://api.example",
		Resume:        true,
	})
	if err == nil || !strings.Contains(err.Error(), "resume ownership mismatch") || resumed.Fixture.TenantNamespace != "tenant-a" {
		t.Fatalf("manifest=%#v err=%v, want missing tenant rejection", resumed, err)
	}
}
