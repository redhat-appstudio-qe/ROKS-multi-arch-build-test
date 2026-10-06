package workflow

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

type fakeStage struct {
	calls     []string
	deadlines map[string]bool
	fail      string
}

func (s *fakeStage) call(ctx context.Context, name string) error {
	s.calls = append(s.calls, name)
	if _, ok := ctx.Deadline(); ok {
		if s.deadlines == nil {
			s.deadlines = map[string]bool{}
		}
		s.deadlines[name] = true
	}
	if s.fail == name {
		return context.Canceled
	}
	return nil
}
func (s *fakeStage) Preflight(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "preflight")
}
func (s *fakeStage) EnsureFixture(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "fixture")
}
func (s *fakeStage) TriggerComponents(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "trigger")
}
func (s *fakeStage) VerifyBuilds(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "builds")
}
func (s *fakeStage) VerifyBuildOutputs(ctx context.Context, _ *model.RunManifest) error {
	return s.call(ctx, "build-outputs")
}

var _ = Describe("Runner", func() {
	It("persists successful phase sequence", func() {
		stage := &fakeStage{}
		runner := Runner{Store: evidence.NewManifestStore(GinkgoT().TempDir()), Stage: stage, Now: func() time.Time { return time.Unix(100, 0) }}
		manifest, err := runner.Run(context.Background(), Options{RunID: "run-1", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant", Application: "app"})
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest.Phase).To(Equal(model.PhaseCompleted))
		Expect(stage.calls).To(Equal([]string{"preflight", "fixture", "trigger", "builds", "build-outputs"}))
	})

	It("persists failure", func() {
		stage := &fakeStage{fail: "build-outputs"}
		runner := Runner{Store: evidence.NewManifestStore(GinkgoT().TempDir()), Stage: stage}
		manifest, err := runner.Run(context.Background(), Options{RunID: "run-2", Provider: "github", ClusterServer: "https://api.example"})
		Expect(err).To(HaveOccurred())
		Expect(manifest.Phase).To(Equal(model.PhaseFailed))
		Expect(manifest.Failure).NotTo(BeNil())
	})

	It("applies phase timeouts", func() {
		stage := &fakeStage{}
		runner := Runner{Store: evidence.NewManifestStore(GinkgoT().TempDir()), Stage: stage}
		_, err := runner.Run(context.Background(), Options{RunID: "run-timeout", Provider: "github", ClusterServer: "https://api.example", Timeouts: PhaseTimeouts{Preflight: time.Minute}})
		Expect(err).NotTo(HaveOccurred())
		Expect(stage.deadlines["preflight"]).To(BeTrue())
	})

	It("rejects resume from failed run", func() {
		store := evidence.NewManifestStore(GinkgoT().TempDir())
		failedStage := &fakeStage{fail: "build-outputs"}
		_, _ = (Runner{Store: store, Stage: failedStage}).Run(context.Background(), Options{RunID: "run-failed", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant"})
		resumed, err := (Runner{Store: store, Stage: &fakeStage{}}).Run(context.Background(), Options{RunID: "run-failed", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant", Resume: true})
		Expect(err).To(MatchError(ContainSubstring("failed run")))
		Expect(resumed.Phase).To(Equal(model.PhaseFailed))
	})

	It("rejects resume with different tenant", func() {
		store := evidence.NewManifestStore(GinkgoT().TempDir())
		runner := Runner{Store: store, Stage: &fakeStage{}}
		_, err := runner.Run(context.Background(), Options{RunID: "run-tenant", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant-a"})
		Expect(err).NotTo(HaveOccurred())
		resumed, err := (Runner{Store: store, Stage: &fakeStage{}}).Run(context.Background(), Options{RunID: "run-tenant", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant-b", Resume: true})
		Expect(err).To(MatchError(ContainSubstring("resume ownership mismatch")))
		Expect(resumed.Fixture.TenantNamespace).To(Equal("tenant-a"))
	})

	It("rejects resume without tenant", func() {
		store := evidence.NewManifestStore(GinkgoT().TempDir())
		runner := Runner{Store: store, Stage: &fakeStage{}}
		_, err := runner.Run(context.Background(), Options{RunID: "run-tenant-empty", Provider: "github", ClusterServer: "https://api.example", FixtureNamespace: "tenant-a"})
		Expect(err).NotTo(HaveOccurred())
		resumed, err := (Runner{Store: store, Stage: &fakeStage{}}).Run(context.Background(), Options{RunID: "run-tenant-empty", Provider: "github", ClusterServer: "https://api.example", Resume: true})
		Expect(err).To(MatchError(ContainSubstring("resume ownership mismatch")))
		Expect(resumed.Fixture.TenantNamespace).To(Equal("tenant-a"))
	})
})
