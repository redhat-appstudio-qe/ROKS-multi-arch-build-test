package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/model"
)

func TestCollectorVerifiesRequiredFailureArtifacts(t *testing.T) {
	root := t.TempDir()
	report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-1", model.RunManifest{RunID: "run-1"}, requiredTestSources())
	if err != nil {
		t.Fatal(err)
	}
	if report.ArtifactPath != filepath.Join(root, "run-1") || len(report.RequiredArtifactNames) != 7 || len(report.SavedArtifactNames) != 7 || report.VerifiedAt.IsZero() {
		t.Fatalf("report = %#v", report)
	}
}

func TestCollectorSuppressesVerifiedReportWhenRequiredSourceFails(t *testing.T) {
	root := t.TempDir()
	sources := requiredTestSources()
	sources[0].Collect = func(context.Context, string) error { return errors.New("api unavailable") }
	report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-2", model.RunManifest{RunID: "run-2"}, sources)
	if err == nil || len(report.CollectionErrors) == 0 || len(report.SavedArtifactNames) >= len(report.RequiredArtifactNames) {
		t.Fatalf("report = %#v err=%v", report, err)
	}
}

func TestCollectorRedactsManifestValues(t *testing.T) {
	root := t.TempDir()
	_, err := (Collector{StateDir: root}).Collect(context.Background(), "run-3", model.RunManifest{RunID: "run-3", ArtifactDirectory: "github-run-14:23_5.10.2026", BuildOutputs: []model.BuildOutputEvidence{{CreatedOutputs: map[string]string{"password": "secret-value"}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "github-run-14:23_5.10.2026", "session", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-value") {
		t.Fatal("secret value leaked")
	}
}

func requiredTestSources() []Source {
	paths := []string{"workload/applications.json", "workload/components.json", "workload/pipelineruns.json", "workload/taskruns.json", "workload/pods.json"}
	sources := make([]Source, 0, len(paths))
	for index, path := range paths {
		path := path
		index := index
		sources = append(sources, Source{Name: filepath.Base(path), Path: path, Collect: func(_ context.Context, root string) error {
			return writeJSON(filepath.Join(root, path), map[string]any{"index": index})
		}})
	}
	return sources
}
