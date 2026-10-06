package evidence

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ManifestStore", func() {
	It("persists phases and latest", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		createdAt := time.Date(2026, 10, 5, 14, 23, 0, 0, time.FixedZone("IDT", 3*60*60))
		manifest := model.RunManifest{RunID: "run-1", Provider: "github", Phase: model.PhasePreflight, CreatedAt: createdAt, TargetClusterServer: "https://api.example", Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
		Expect(store.Create(&manifest)).To(Succeed())
		manifest, err := store.Load("run-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest.ArtifactDirectory).To(Equal("github-run-14:23_5.10.2026"))
		Expect(store.Transition(&manifest, model.PhaseFixtureReady)).To(Succeed())
		loaded, err := store.Load("run-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Phase).To(Equal(model.PhaseFixtureReady))
		_, err = store.Latest()
		Expect(err).To(HaveOccurred())
		for _, name := range []string{"manifest.json", "status.json"} {
			_, err = os.Stat(filepath.Join(store.RunDirFor(loaded), name))
			Expect(err).NotTo(HaveOccurred())
		}
		loaded.Phase = model.PhaseCompleted
		Expect(store.PublishLatest(loaded)).To(Succeed())
		latest, err := store.Latest()
		Expect(err).NotTo(HaveOccurred())
		Expect(latest).To(Equal("run-1"))
		latestInfo, err := os.Stat(filepath.Join(store.Root, "latest"))
		Expect(err).NotTo(HaveOccurred())
		Expect(latestInfo.IsDir()).To(BeTrue())
	})

	It("writes identity and redacts credentials", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		manifest := model.RunManifest{RunID: "run-2", Phase: model.PhasePreflight, PipelineRuns: []model.PipelineRunIdentity{{Namespace: "tenant", Name: "build", UID: types.UID("uid-1"), StartedAt: time.Now().UTC()}}, BuildOutputs: []model.BuildOutputEvidence{{CreatedOutputs: map[string]string{"password": "secret-value"}}}}
		Expect(store.Create(&manifest)).To(Succeed())
		data, err := os.ReadFile(filepath.Join(store.RunDirFor(manifest), "manifest.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("do-not-write"))
		Expect(string(data)).To(ContainSubstring("uid-1"))
	})

	It("preserves latest until new run completes", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		first := model.RunManifest{RunID: "run-first", Provider: "gitlab", CreatedAt: time.Date(2026, 10, 5, 14, 24, 0, 0, time.UTC), Phase: model.PhaseCompleted}
		Expect(store.Create(&first)).To(Succeed())
		Expect(store.PublishLatest(first)).To(Succeed())
		second := model.RunManifest{RunID: "run-second", Provider: "gitlab", CreatedAt: time.Date(2026, 10, 5, 14, 25, 0, 0, time.UTC), Phase: model.PhasePreflight}
		Expect(store.Create(&second)).To(Succeed())
		latest, err := store.Latest()
		Expect(err).NotTo(HaveOccurred())
		Expect(latest).To(Equal(first.RunID))
	})

	It("rejects invalid phase transition", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		manifest := model.RunManifest{RunID: "run-3", Phase: model.PhasePreflight}
		Expect(store.Create(&manifest)).To(Succeed())
		Expect(store.Transition(&manifest, model.PhaseBuildOutputsVerified)).To(HaveOccurred())
	})
})
