package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/model"
)

var _ = ginkgo.Describe("Collector", func() {
	ginkgo.It("verifies required failure artifacts", func() {
		root := ginkgo.GinkgoT().TempDir()
		report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-1", model.RunManifest{RunID: "run-1"}, requiredTestSources())
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(report.ArtifactPath).To(Equal(filepath.Join(root, "run-1")))
		Expect(report.RequiredArtifactNames).To(HaveLen(7))
		Expect(report.SavedArtifactNames).To(HaveLen(7))
		Expect(report.VerifiedAt).NotTo(BeZero())
	})

	ginkgo.It("suppresses verified report when required source fails", func() {
		root := ginkgo.GinkgoT().TempDir()
		sources := requiredTestSources()
		sources[0].Collect = func(context.Context, string) error { return errors.New("api unavailable") }
		report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-2", model.RunManifest{RunID: "run-2"}, sources)
		Expect(err).To(HaveOccurred())
		Expect(report.CollectionErrors).NotTo(BeEmpty())
		Expect(len(report.SavedArtifactNames)).To(BeNumerically("<", len(report.RequiredArtifactNames)))
	})

	ginkgo.It("redacts manifest values", func() {
		root := ginkgo.GinkgoT().TempDir()
		_, err := (Collector{StateDir: root}).Collect(context.Background(), "run-3", model.RunManifest{RunID: "run-3", ArtifactDirectory: "github-run-14:23_5.10.2026", BuildOutputs: []model.BuildOutputEvidence{{CreatedOutputs: map[string]string{"password": "secret-value"}}}}, nil)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		data, err := os.ReadFile(filepath.Join(root, "github-run-14:23_5.10.2026", "session", "manifest.json"))
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(string(data)).NotTo(ContainSubstring("secret-value"))
	})

	ginkgo.It("requires failed-build directory when failures exist", func() {
		root := ginkgo.GinkgoT().TempDir()
		sources := requiredTestSources()
		sources = append(sources, Source{Name: "failed-builds", Path: "failed-builds/", Collect: func(_ context.Context, root string) error {
			if err := os.MkdirAll(filepath.Join(root, "failed-builds"), 0o750); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, "failed-builds", "build.log"), []byte("failed"), 0o640)
		}})
		manifest := model.RunManifest{
			RunID: "run-4",
			FailedBuildLogs: []model.FailedBuildLog{{
				PipelineRunName: "pr-1",
				TaskRunName:     "tr-1",
			}},
		}

		report, err := (Collector{StateDir: root}).CollectAndVerify(context.Background(), "run-4", manifest, sources)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(report.RequiredArtifactNames).To(HaveLen(8))
		Expect(report.SavedArtifactNames).To(HaveLen(8))
	})
})

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
