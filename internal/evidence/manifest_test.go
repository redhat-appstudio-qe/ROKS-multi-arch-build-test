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
	manifest := model.RunManifest{RunID: "run-1", Provider: "github", Phase: model.PhasePreflight, TargetClusterServer: "https://api.example", Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
	if err := store.Create(manifest); err != nil {
		t.Fatal(err)
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
	if latest, err := store.Latest(); err != nil || latest != "run-1" {
		t.Fatalf("latest = %q, %v", latest, err)
	}
	for _, name := range []string{"manifest.json", "status.json"} {
		if _, err := os.Stat(filepath.Join(store.RunDir("run-1"), name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManifestStoreWritesIdentityAndRedactsCredentials(t *testing.T) {
	store := NewManifestStore(t.TempDir())
	manifest := model.RunManifest{RunID: "run-2", Phase: model.PhasePreflight, PipelineRuns: []model.PipelineRunIdentity{{Namespace: "tenant", Name: "build", UID: types.UID("uid-1"), StartedAt: time.Now().UTC()}}, Archive: []model.ArchiveEvidence{{RawResponse: map[string]any{"token": "do-not-write", "status": "Succeeded"}}}}
	if err := store.Create(manifest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.RunDir("run-2"), "manifest.json"))
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

func TestInvalidPhaseTransitionIsRejected(t *testing.T) {
	store := NewManifestStore(t.TempDir())
	manifest := model.RunManifest{RunID: "run-3", Phase: model.PhasePreflight}
	if err := store.Create(manifest); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(&manifest, model.PhaseImagesVerified); err == nil {
		t.Fatal("expected invalid transition")
	}
}
