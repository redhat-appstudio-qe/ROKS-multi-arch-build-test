package model

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type Phase string

const (
	PhasePreflight            Phase = "preflight"
	PhaseFixtureReady         Phase = "fixture-ready"
	PhaseTriggered            Phase = "triggered"
	PhaseBuildsVerified       Phase = "builds-verified"
	PhaseBuildOutputsVerified Phase = "build-outputs-verified"
	PhaseCompleted            Phase = "completed"
	PhaseFailed               Phase = "failed"
)

var phaseOrder = []Phase{
	PhasePreflight, PhaseFixtureReady, PhaseTriggered, PhaseBuildsVerified,
	PhaseBuildOutputsVerified, PhaseCompleted,
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
	RepositoryOwner string   `json:"repositoryOwner"`
	RepositoryName  string   `json:"repositoryName"`
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

type BuildOutputEvidence struct {
	Component      string              `json:"component"`
	PipelineRun    PipelineRunIdentity `json:"pipelineRun"`
	Platforms      []string            `json:"platforms"`
	CreatedOutputs map[string]string   `json:"createdOutputs,omitempty"`
	OutputDigests  map[string]string   `json:"outputDigests,omitempty"`
	VerifiedAt     time.Time           `json:"verifiedAt"`
}

type FailedBuildLog struct {
	PipelineRunName  string `json:"pipelineRunName"`
	TaskRunName      string `json:"taskRunName"`
	TaskName         string `json:"taskName"`
	StepContainer    string `json:"stepContainer"`
	ExitCode         int64  `json:"exitCode"`
	ConditionMessage string `json:"conditionMessage"`
	LogPath          string `json:"logPath,omitempty"`
}

type Failure struct {
	Phase   Phase  `json:"phase"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type FailureArtifactReport struct {
	ArtifactPath          string    `json:"artifactPath"`
	RequiredArtifactNames []string  `json:"requiredArtifactNames"`
	SavedArtifactNames    []string  `json:"savedArtifactNames"`
	CollectionErrors      []string  `json:"collectionErrors,omitempty"`
	VerifiedAt            time.Time `json:"verifiedAt"`
}

type RunManifest struct {
	RunID               string                 `json:"runID"`
	ArtifactDirectory   string                 `json:"artifactDirectory"`
	Provider            string                 `json:"provider"`
	TargetClusterServer string                 `json:"targetClusterServer"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
	Phase               Phase                  `json:"phase"`
	Fixture             FixtureIdentity        `json:"fixture"`
	TriggerCommits      []TriggerCommit        `json:"triggerCommits,omitempty"`
	PipelineRuns        []PipelineRunIdentity  `json:"pipelineRuns,omitempty"`
	BuildOutputs        []BuildOutputEvidence  `json:"buildOutputs,omitempty"`
	FailedBuildLogs     []FailedBuildLog       `json:"failedBuildLogs,omitempty"`
	Failure             *Failure               `json:"failure,omitempty"`
	FailureArtifacts    *FailureArtifactReport `json:"failureArtifacts,omitempty"`
}
