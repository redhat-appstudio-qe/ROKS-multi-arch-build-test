package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	"k8s.io/apimachinery/pkg/types"
)

func TestManifestStorePersistsPhasesAndLatest(t *testing.T) {
	store := NewManifestStore(t.TempDir())
	createdAt := time.Date(2026, 10, 5, 14, 23, 0, 0, time.FixedZone("IDT", 3*60*60))
	manifest := model.RunManifest{RunID: "run-1", Provider: "github", Phase: model.PhasePreflight, CreatedAt: createdAt, TargetClusterServer: "https://api.example", Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
	if err := store.Create(&manifest); err != nil {
		t.Fatal(err)
	}
	manifest, err := store.Load("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ArtifactDirectory != "github-run-14:23_5.10.2026" {
		t.Fatalf("artifact directory = %q", manifest.ArtifactDirectory)
	}
	if err := store.Transition(&manifest, model.PhaseFixtureReady); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Phase != model.PhaseFixtureReady {
		t.Fatalf("phase = %q", loaded.Phase)
	}
	if _, err := store.Latest(); err == nil {
		t.Fatal("latest exists before a completed run is published")
	}
	for _, name := range []string{"manifest.json", "status.json"} {
		if _, err := os.Stat(filepath.Join(store.RunDirFor(loaded), name)); err != nil {
			t.Fatal(err)
		}
	}
	loaded.Phase = model.PhaseCompleted
	if err := store.PublishLatest(loaded); err != nil {
		t.Fatal(err)
	}
	if latest, err := store.Latest(); err != nil || latest != "run-1" {
		t.Fatalf("latest = %q, %v", latest, err)
	}
	latestInfo, err := os.Stat(filepath.Join(store.Root, "latest"))
	if err != nil || !latestInfo.IsDir() {
		t.Fatalf("latest is not a directory: %v", err)
	}
}

func TestManifestStoreWritesIdentityAndRedactsCredentials(t *testing.T) {
	store := NewManifestStore(t.TempDir())
	manifest := model.RunManifest{RunID: "run-2", Phase: model.PhasePreflight, PipelineRuns: []model.PipelineRunIdentity{{Namespace: "tenant", Name: "build", UID: types.UID("uid-1"), StartedAt: time.Now().UTC()}}, BuildOutputs: []model.BuildOutputEvidence{{CreatedOutputs: map[string]string{"password": "secret-value"}}}}
	if err := store.Create(&manifest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.RunDirFor(manifest), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "do-not-write") {
		t.Fatalf("secret value leaked: %s", data)
	}
	if !strings.Contains(string(data), "uid-1") {
		t.Fatalf("identity missing: %s", data)
	}
}

func TestManifestStorePreservesLatestUntilNewRunCompletes(t *testing.T) {
	store := NewManifestStore(t.TempDir())
	first := model.RunManifest{RunID: "run-first", Provider: "gitlab", CreatedAt: time.Date(2026, 10, 5, 14, 24, 0, 0, time.UTC), Phase: model.PhaseCompleted}
	if err := store.Create(&first); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishLatest(first); err != nil {
		t.Fatal(err)
	}
	second := model.RunManifest{RunID: "run-second", Provider: "gitlab", CreatedAt: time.Date(2026, 10, 5, 14, 25, 0, 0, time.UTC), Phase: model.PhasePreflight}
	if err := store.Create(&second); err != nil {
		t.Fatal(err)
	}
	if latest, err := store.Latest(); err != nil || latest != first.RunID {
		t.Fatalf("latest after new run creation = %q, %v", latest, err)
	}
}

func TestInvalidPhaseTransitionIsRejected(t *testing.T) {
	store := NewManifestStore(t.TempDir())
	manifest := model.RunManifest{RunID: "run-3", Phase: model.PhasePreflight}
	if err := store.Create(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(&manifest, model.PhaseBuildOutputsVerified); err == nil {
		t.Fatal("expected invalid transition")
	}
}
