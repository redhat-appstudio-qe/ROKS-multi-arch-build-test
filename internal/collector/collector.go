package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

type Source struct {
	Name     string
	Optional bool
	Collect  func(context.Context, string) error
}

type Attempt struct {
	Name      string    `json:"name"`
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

func (c Collector) Collect(ctx context.Context, runID string, manifest model.RunManifest, sources []Source) (Report, error) {
	if c.StateDir == "" || runID == "" {
		return Report{}, fmt.Errorf("state directory and run ID are required")
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	root := filepath.Join(c.StateDir, runID)
	for _, dir := range []string{"session", "controllers", "workload", "external", "archive"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			return Report{}, fmt.Errorf("create collection directory: %w", err)
		}
	}
	if err := writeJSON(filepath.Join(root, "session", "manifest.json"), manifest); err != nil {
		return Report{}, err
	}
	report := Report{RunID: runID, StartedAt: now().UTC()}
	for _, source := range sources {
		attempt := Attempt{Name: source.Name, StartedAt: now().UTC()}
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
	report.EndedAt = now().UTC()
	if err := writeJSON(filepath.Join(root, "collection-report.json"), report); err != nil {
		return Report{}, err
	}
	return report, nil
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
