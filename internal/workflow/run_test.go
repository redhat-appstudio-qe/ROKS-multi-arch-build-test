package workflow

import (
	"context"
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
func (s *fakeStage) CaptureBaseline(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "baseline")
}
func (s *fakeStage) TriggerComponents(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "trigger")
}
func (s *fakeStage) VerifyBuilds(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "builds")
}
func (s *fakeStage) VerifyImages(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "images")
}
func (s *fakeStage) ObservePruning(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "pruning")
}
func (s *fakeStage) VerifyArchive(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "archive")
}

func TestRunnerPersistsSuccessfulPhaseSequence(t *testing.T) {
	stage := &fakeStage{}
	runner := Runner{Store: evidence.NewManifestStore(t.TempDir()), Stage: stage, Now: func() time.Time { return time.Unix(100, 0) }}
	manifest, err := runner.Run(context.Background(), Options{RunID: "run-1", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant", Application: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Phase != model.PhaseCompleted || len(stage.calls) != 8 {
		t.Fatalf("phase=%q calls=%v", manifest.Phase, stage.calls)
	}
}

func TestRunnerPersistsFailure(t *testing.T) {
	stage := &fakeStage{fail: "images"}
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
