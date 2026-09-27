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

func TestCollectorContinuesAfterOptionalSourceFailure(t *testing.T) {
	root := t.TempDir()
	report, err := (Collector{StateDir: root}).Collect(context.Background(), "run-1", model.RunManifest{RunID: "run-1"}, []Source{
		{Name: "workload", Collect: func(context.Context, string) error { return errors.New("api unavailable") }},
		{Name: "registry", Optional: true, Collect: func(context.Context, string) error { return nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Attempts) != 2 || report.Attempts[0].Status != "error" || report.Attempts[1].Status != "success" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if _, err := os.Stat(filepath.Join(root, "run-1", "collection-report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorRedactsManifestValues(t *testing.T) {
	root := t.TempDir()
	_, err := (Collector{StateDir: root}).Collect(context.Background(), "run-2", model.RunManifest{RunID: "run-2", Archive: []model.ArchiveEvidence{{RawResponse: map[string]any{"password": "secret-value"}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "run-2", "session", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-value") {
		t.Fatal("secret value leaked")
	}
}
