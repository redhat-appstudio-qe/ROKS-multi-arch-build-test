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
		Expect(manifest.ArtifactDirectory).To(Equal("github-run-14:23_5.10.2026_run-1"))
		Expect(store.Transition(&manifest, model.PhaseFixtureReady)).To(Succeed())
		loaded, err := store.Load("run-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Phase).To(Equal(model.PhaseFixtureReady))
		latest, err := store.Latest()
		Expect(err).NotTo(HaveOccurred())
		Expect(latest).To(Equal("run-1"))
		for _, name := range []string{"manifest.json", "status.json"} {
			_, err = os.Stat(filepath.Join(store.RunDirFor(loaded), name))
			Expect(err).NotTo(HaveOccurred())
		}
		latestInfo, err := os.Lstat(filepath.Join(store.Root, "latest"))
		Expect(err).NotTo(HaveOccurred())
		Expect(latestInfo.Mode() & os.ModeSymlink).NotTo(BeZero())
		latestTarget, err := os.Readlink(filepath.Join(store.Root, "latest"))
		Expect(err).NotTo(HaveOccurred())
		Expect(latestTarget).To(Equal(manifest.ArtifactDirectory))
		Expect(filepath.IsAbs(latestTarget)).To(BeFalse())
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

	It("tracks the active run through failure and success while preserving prior artifacts", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		createdAt := time.Date(2026, 10, 5, 14, 24, 0, 0, time.UTC)
		first := model.RunManifest{RunID: "run-first", Provider: "gitlab", CreatedAt: createdAt, Phase: model.PhasePreflight}
		Expect(store.Create(&first)).To(Succeed())
		expectLatestRun(store, "run-first")
		firstArtifact := filepath.Join(store.RunDirFor(first), "failure-note.txt")
		Expect(os.WriteFile(firstArtifact, []byte("first run"), 0o640)).To(Succeed())
		latestArtifact, err := os.ReadFile(filepath.Join(store.Root, "latest", "failure-note.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(latestArtifact)).To(Equal("first run"))
		Expect(store.Transition(&first, model.PhaseFailed)).To(Succeed())
		expectLatestRun(store, "run-first")

		second := model.RunManifest{RunID: "run-second", Provider: "gitlab", CreatedAt: createdAt, Phase: model.PhasePreflight}
		Expect(store.Create(&second)).To(Succeed())
		expectLatestRun(store, "run-second")
		firstContents, err := os.ReadFile(firstArtifact)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(firstContents)).To(Equal("first run"))

		for _, phase := range []model.Phase{model.PhaseFixtureReady, model.PhaseTriggered, model.PhaseBuildsVerified, model.PhaseBuildOutputsVerified, model.PhaseCompleted} {
			Expect(store.Transition(&second, phase)).To(Succeed())
		}
		expectLatestRun(store, "run-second")
	})

	It("does not move latest back to an older run created later", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		newer := model.RunManifest{RunID: "run-newer", Provider: "github", CreatedAt: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC), Phase: model.PhasePreflight}
		Expect(store.Create(&newer)).To(Succeed())
		older := model.RunManifest{RunID: "run-older", Provider: "github", CreatedAt: newer.CreatedAt.Add(-time.Minute), Phase: model.PhasePreflight}
		Expect(store.Create(&older)).To(Succeed())
		expectLatestRun(store, "run-newer")
	})

	It("moves aside legacy latest directories before switching the pointer", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		first := model.RunManifest{RunID: "run-first", Provider: "github", CreatedAt: time.Date(2026, 10, 5, 14, 24, 0, 0, time.UTC), Phase: model.PhasePreflight}
		Expect(store.Create(&first)).To(Succeed())
		latestPath := filepath.Join(store.Root, "latest")
		Expect(os.Remove(latestPath)).To(Succeed())
		Expect(os.Mkdir(latestPath, 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(latestPath, "legacy-note.txt"), []byte("preserve"), 0o640)).To(Succeed())

		second := model.RunManifest{RunID: "run-second", Provider: "github", CreatedAt: first.CreatedAt.Add(time.Second), Phase: model.PhasePreflight}
		Expect(store.Create(&second)).To(Succeed())
		expectLatestRun(store, "run-second")
		info, err := os.Lstat(latestPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode() & os.ModeSymlink).NotTo(BeZero())
		legacyPaths, err := filepath.Glob(filepath.Join(store.Root, ".latest-legacy-*"))
		Expect(err).NotTo(HaveOccurred())
		Expect(legacyPaths).To(HaveLen(1))
		legacyContents, err := os.ReadFile(filepath.Join(legacyPaths[0], "legacy-note.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(legacyContents)).To(Equal("preserve"))
	})

	It("rejects artifact directories outside the state root without changing latest", func() {
		root := GinkgoT().TempDir()
		store := NewManifestStore(filepath.Join(root, "runs"))
		first := model.RunManifest{RunID: "run-first", Provider: "github", CreatedAt: time.Now().UTC(), Phase: model.PhasePreflight}
		Expect(store.Create(&first)).To(Succeed())

		unsafe := model.RunManifest{RunID: "run-unsafe", ArtifactDirectory: "../outside", Phase: model.PhasePreflight}
		Expect(store.Create(&unsafe)).To(MatchError(ContainSubstring("single directory name")))
		Expect(store.SetLatest(unsafe)).To(MatchError(ContainSubstring("single directory name")))
		expectLatestRun(store, "run-first")
		_, err := os.Stat(filepath.Join(root, "outside"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("rejects invalid phase transition", func() {
		store := NewManifestStore(GinkgoT().TempDir())
		manifest := model.RunManifest{RunID: "run-3", Phase: model.PhasePreflight}
		Expect(store.Create(&manifest)).To(Succeed())
		Expect(store.Transition(&manifest, model.PhaseBuildOutputsVerified)).To(HaveOccurred())
	})
})

func expectLatestRun(store ManifestStore, runID string) {
	latest, err := store.Latest()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(latest).Should(Equal(runID))
}
