package model

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type Phase string

const (
	PhasePreflight       Phase = "preflight"
	PhaseFixtureReady    Phase = "fixture-ready"
	PhaseTriggered       Phase = "triggered"
	PhaseBuildsVerified  Phase = "builds-verified"
	PhaseImagesVerified  Phase = "images-verified"
	PhasePruningWait     Phase = "pruning-wait"
	PhaseArchiveVerified Phase = "archive-verified"
	PhaseCompleted       Phase = "completed"
	PhaseFailed          Phase = "failed"
)

var phaseOrder = []Phase{
	PhasePreflight, PhaseFixtureReady, PhaseTriggered, PhaseBuildsVerified,
	PhaseImagesVerified, PhasePruningWait, PhaseArchiveVerified, PhaseCompleted,
}

func IsTerminalPhase(phase Phase) bool {
	return phase == PhaseCompleted || phase == PhaseFailed
}

func ValidateTransition(from, to Phase) error {
	if to == PhaseFailed {
		if from == PhaseCompleted {
			return fmt.Errorf("cannot fail a completed run")
		}
		return nil
	}
	if from == PhaseFailed || from == PhaseCompleted {
		return fmt.Errorf("cannot transition from terminal phase %q", from)
	}
	for index, phase := range phaseOrder {
		if phase == from {
			if index+1 < len(phaseOrder) && phaseOrder[index+1] == to {
				return nil
			}
			return fmt.Errorf("invalid transition %q -> %q", from, to)
		}
	}
	if from == "" && to == PhasePreflight {
		return nil
	}
	return fmt.Errorf("unknown phase transition %q -> %q", from, to)
}

type FixtureIdentity struct {
	TenantNamespace string   `json:"tenantNamespace"`
	Application     string   `json:"application"`
	Components      []string `json:"components"`
	Repository      string   `json:"repository"`
	Branch          string   `json:"branch"`
}

type PipelineRunIdentity struct {
	Namespace   string    `json:"namespace"`
	Name        string    `json:"name"`
	UID         types.UID `json:"uid"`
	Component   string    `json:"component"`
	SourceSHA   string    `json:"sourceSHA"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	Succeeded   bool      `json:"succeeded"`
}

type TriggerCommit struct {
	Component string    `json:"component"`
	SHA       string    `json:"sha"`
	URL       string    `json:"url,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type ImageEvidence struct {
	Component    string         `json:"component"`
	Reference    string         `json:"reference"`
	Digest       string         `json:"digest"`
	OS           string         `json:"os"`
	Architecture string         `json:"architecture"`
	Registry     map[string]any `json:"registry,omitempty"`
}

type PruningObservation struct {
	PipelineRun   PipelineRunIdentity `json:"pipelineRun"`
	ObservedAt    time.Time           `json:"observedAt"`
	DisappearedAt time.Time           `json:"disappearedAt"`
	Normal        bool                `json:"normal"`
	Details       map[string]any      `json:"details,omitempty"`
}

type ArchiveEvidence struct {
	Endpoint    string          `json:"endpoint"`
	QueriedAt   time.Time       `json:"queriedAt"`
	RawResponse map[string]any  `json:"rawResponse,omitempty"`
	Matched     bool            `json:"matched"`
	Comparisons map[string]bool `json:"comparisons"`
}

type Failure struct {
	Phase   Phase  `json:"phase"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type CollectionReport struct {
	Path      string    `json:"path"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type RunManifest struct {
	RunID                string                `json:"runID"`
	Provider             string                `json:"provider"`
	TargetClusterServer  string                `json:"targetClusterServer"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
	Phase                Phase                 `json:"phase"`
	Fixture              FixtureIdentity       `json:"fixture"`
	BaselinePipelineRuns []PipelineRunIdentity `json:"baselinePipelineRuns,omitempty"`
	TriggerCommits       []TriggerCommit       `json:"triggerCommits,omitempty"`
	PipelineRuns         []PipelineRunIdentity `json:"pipelineRuns,omitempty"`
	Images               []ImageEvidence       `json:"images,omitempty"`
	Pruning              []PruningObservation  `json:"pruning,omitempty"`
	Archive              []ArchiveEvidence     `json:"archive,omitempty"`
	Failure              *Failure              `json:"failure,omitempty"`
	Collection           *CollectionReport     `json:"collection,omitempty"`
}
