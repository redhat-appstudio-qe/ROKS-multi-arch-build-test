package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

var requiredArtifactNames = []string{
	"session/manifest.json",
	"workload/applications.json",
	"workload/components.json",
	"workload/pipelineruns.json",
	"workload/taskruns.json",
	"workload/taskruns/",
	"workload/pods.json",
	"collection-report.json",
}

type Source struct {
	Name     string
	Path     string
	Optional bool
	Collect  func(context.Context, string) error
}

type Attempt struct {
	Name      string    `json:"name"`
	Path      string    `json:"path,omitempty"`
	Optional  bool      `json:"optional,omitempty"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type Report struct {
	RunID     string    `json:"runID"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
	Attempts  []Attempt `json:"attempts"`
}

type Collector struct {
	StateDir string
	Now      func() time.Time
}

func RequiredArtifactNames() []string {
	return append([]string(nil), requiredArtifactNames...)
}

func RequiredArtifactNamesForManifest(manifest model.RunManifest) []string {
	names := RequiredArtifactNames()
	if len(manifest.FailedBuildLogs) > 0 {
		names = append(names, "failed-builds/")
	}
	return names
}

func (c Collector) Collect(ctx context.Context, runID string, manifest model.RunManifest, sources []Source) (Report, error) {
	if c.StateDir == "" || runID == "" {
		return Report{}, fmt.Errorf("state directory and run ID are required")
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	root := c.runRoot(runID, manifest)
	if err := os.MkdirAll(filepath.Join(root, "workload"), 0o750); err != nil {
		return Report{}, fmt.Errorf("create collection directory: %w", err)
	}
	if err := writeJSON(filepath.Join(root, "session", "manifest.json"), manifest); err != nil {
		return Report{}, err
	}
	report := Report{RunID: runID, StartedAt: now().UTC(), Attempts: []Attempt{{Name: "manifest", Path: "session/manifest.json", Status: "success", StartedAt: now().UTC(), EndedAt: now().UTC()}}}
	for _, source := range sources {
		attempt := Attempt{Name: source.Name, Path: source.Path, Optional: source.Optional, StartedAt: now().UTC()}
		if source.Collect == nil {
			attempt.Status = "omitted"
		} else if err := source.Collect(ctx, root); err != nil {
			attempt.Status = "error"
			attempt.Error = err.Error()
		} else {
			attempt.Status = "success"
		}
		attempt.EndedAt = now().UTC()
		report.Attempts = append(report.Attempts, attempt)
	}
	if len(manifest.FailedBuildLogs) > 0 && !successfulAttempt(report, "failed-builds/") {
		failedBuildsPath := filepath.Join(root, "failed-builds")
		if info, err := os.Stat(failedBuildsPath); err == nil && info.IsDir() {
			report.Attempts = append(report.Attempts, Attempt{Name: "failed-builds", Path: "failed-builds/", Status: "success", StartedAt: now().UTC(), EndedAt: now().UTC()})
		}
	}
	report.EndedAt = now().UTC()
	report.Attempts = append(report.Attempts, Attempt{Name: "collection-report", Path: "collection-report.json", Status: "success", StartedAt: report.EndedAt, EndedAt: report.EndedAt})
	if err := writeJSON(filepath.Join(root, "collection-report.json"), report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (c Collector) CollectAndVerify(ctx context.Context, runID string, manifest model.RunManifest, sources []Source) (model.FailureArtifactReport, error) {
	report, err := c.Collect(ctx, runID, manifest, sources)
	root := c.runRoot(runID, manifest)
	artifactReport := model.FailureArtifactReport{ArtifactPath: root, RequiredArtifactNames: RequiredArtifactNamesForManifest(manifest)}
	if err != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, err.Error())
		return artifactReport, err
	}
	storedReport, err := ReadReport(filepath.Join(root, "collection-report.json"))
	if err != nil {
		artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, "collection-report.json: "+err.Error())
		return artifactReport, fmt.Errorf("read collection report: %w", err)
	}
	report = storedReport
	for _, attempt := range report.Attempts {
		if attempt.Status == "error" && !attempt.Optional {
			artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, attempt.Name+": "+attempt.Error)
		}
	}
	for _, name := range artifactReport.RequiredArtifactNames {
		path := filepath.Join(root, name)
		var readErr error
		if strings.HasSuffix(name, "/") {
			info, statErr := os.Stat(path)
			if statErr != nil || !info.IsDir() {
				readErr = statErr
				if readErr == nil {
					readErr = fmt.Errorf("not a directory")
				}
			}
		} else {
			_, readErr = os.ReadFile(path)
		}
		if readErr != nil {
			artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, name+": "+readErr.Error())
			continue
		}
		if name != "session/manifest.json" && !successfulAttempt(report, name) {
			artifactReport.CollectionErrors = append(artifactReport.CollectionErrors, name+": collection was not successful")
			continue
		}
		artifactReport.SavedArtifactNames = append(artifactReport.SavedArtifactNames, name)
	}
	if len(artifactReport.CollectionErrors) > 0 {
		return artifactReport, fmt.Errorf("failure artifacts incomplete under %s: %v", root, artifactReport.CollectionErrors)
	}
	artifactReport.VerifiedAt = time.Now().UTC()
	return artifactReport, nil
}

func (c Collector) runRoot(runID string, manifest model.RunManifest) string {
	directory := manifest.ArtifactDirectory
	if directory == "" {
		directory = runID
	}
	return filepath.Join(c.StateDir, directory)
}

func successfulAttempt(report Report, path string) bool {
	for _, attempt := range report.Attempts {
		if attempt.Path == path && attempt.Status == "success" {
			return true
		}
	}
	return false
}

func writeJSON(path string, value any) error {
	data, err := evidence.RedactedJSON(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}

func ReadReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}
