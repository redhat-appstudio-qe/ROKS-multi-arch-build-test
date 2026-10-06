package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

const TriggerCommentFormat = "# konflux-test trigger %s %d"

type Confirmation interface {
	Confirm(context.Context, string) (bool, error)
}

type Stage interface {
	Preflight(context.Context, *model.RunManifest) error
	EnsureFixture(context.Context, *model.RunManifest) error
	TriggerComponents(context.Context, *model.RunManifest) error
	VerifyBuilds(context.Context, *model.RunManifest) error
	VerifyBuildOutputs(context.Context, *model.RunManifest) error
}

type Options struct {
	RunID            string
	Provider         string
	ClusterServer    string
	Resume           bool
	StateDir         string
	FixtureNamespace string
	Application      string
	Timeouts         PhaseTimeouts
}

type PhaseTimeouts struct {
	Preflight time.Duration
	Build     time.Duration
}

type Runner struct {
	Store evidence.ManifestStore
	Stage Stage
	Now   func() time.Time
}

func (r Runner) Run(ctx context.Context, options Options) (model.RunManifest, error) {
	if r.Stage == nil {
		return model.RunManifest{}, fmt.Errorf("workflow stage is required")
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	runID := options.RunID
	if runID == "" {
		runID = "run-" + uuid.NewString()
	}
	var manifest model.RunManifest
	if options.Resume {
		if !validRunID(runID) {
			return model.RunManifest{}, fmt.Errorf("invalid resume run ID %q", runID)
		}
		var err error
		manifest, err = r.Store.Load(runID)
		if err != nil {
			return model.RunManifest{}, fmt.Errorf("load run %s: %w", runID, err)
		}
		if manifest.Provider != options.Provider || manifest.TargetClusterServer != options.ClusterServer {
			return manifest, fmt.Errorf("resume ownership mismatch")
		}
		if manifest.Fixture.TenantNamespace == "" || options.FixtureNamespace == "" || manifest.Fixture.TenantNamespace != options.FixtureNamespace {
			return manifest, fmt.Errorf("resume ownership mismatch")
		}
		if manifest.Phase == model.PhaseFailed {
			return manifest, fmt.Errorf("cannot resume failed run %s; start a new run", runID)
		}
	} else {
		manifest = model.RunManifest{RunID: runID, Provider: options.Provider, TargetClusterServer: options.ClusterServer, Phase: model.PhasePreflight, CreatedAt: now().UTC(), Fixture: model.FixtureIdentity{TenantNamespace: options.FixtureNamespace, Application: options.Application}}
		if err := r.Store.Create(&manifest); err != nil {
			return manifest, err
		}
	}
	steps := []struct {
		phase   model.Phase
		timeout time.Duration
		run     func(context.Context, *model.RunManifest) error
	}{
		{model.PhasePreflight, options.Timeouts.Preflight, r.Stage.Preflight},
		{model.PhaseFixtureReady, options.Timeouts.Preflight, r.Stage.EnsureFixture},
		{model.PhaseTriggered, options.Timeouts.Build, r.Stage.TriggerComponents},
		{model.PhaseBuildsVerified, options.Timeouts.Build, r.Stage.VerifyBuilds},
		{model.PhaseBuildOutputsVerified, options.Timeouts.Build, r.Stage.VerifyBuildOutputs},
		{model.PhaseCompleted, 0, func(context.Context, *model.RunManifest) error { return nil }},
	}
	for index, step := range steps {
		if shouldSkip(manifest.Phase, step.phase, index, steps) {
			continue
		}
		stepContext, cancel := contextWithTimeout(ctx, step.timeout)
		err := step.run(stepContext, &manifest)
		cancel()
		if err != nil {
			manifest.Failure = &model.Failure{Phase: step.phase, Reason: failureReason(step.phase), Message: err.Error()}
			_ = r.Store.Transition(&manifest, model.PhaseFailed)
			return manifest, err
		}
		if step.phase != manifest.Phase {
			if err := r.Store.Transition(&manifest, step.phase); err != nil {
				return manifest, err
			}
		}
	}
	if err := r.Store.PublishLatest(manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func shouldSkip(current, target model.Phase, index int, _ []struct {
	phase   model.Phase
	timeout time.Duration
	run     func(context.Context, *model.RunManifest) error
}) bool {
	if current == model.PhaseFailed || current == model.PhaseCompleted {
		return true
	}
	start := map[model.Phase]int{
		model.PhasePreflight:            0,
		model.PhaseFixtureReady:         1,
		model.PhaseTriggered:            2,
		model.PhaseBuildsVerified:       3,
		model.PhaseBuildOutputsVerified: 4,
		model.PhaseCompleted:            5,
	}
	return index < start[current] || target == ""
}

func contextWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

func failureReason(phase model.Phase) string {
	return strings.TrimSuffix(string(phase), "-wait") + "-failed"
}

func validRunID(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' && char != '.' {
			return false
		}
	}
	return true
}
