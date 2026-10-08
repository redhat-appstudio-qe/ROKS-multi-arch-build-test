package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/cleanup"
	"github.com/redhat-appstudio/konflux-test/internal/collector"
	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

type fakePrompt struct {
	approved bool
	calls    int
}

func (p *fakePrompt) Confirm(context.Context, string) (bool, error) {
	p.calls++
	return p.approved, nil
}

var _ = Describe("Lifecycle", func() {
	It("collects failure artifacts before prompt", func() {
		stateDir := GinkgoT().TempDir()
		client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespaceObject("tenant", "run-1"))
		prompt := &fakePrompt{approved: false}
		manifest := model.RunManifest{RunID: "run-1", Phase: model.PhaseFailed, Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
		sources := lifecycleTestSources()
		lifecycle := Lifecycle{Store: evidence.NewManifestStore(stateDir), Namespace: cleanup.NamespaceService{Dynamic: client}, Collector: collector.Collector{StateDir: stateDir}, Prompt: prompt, Sources: sources}
		if err := lifecycle.Finalize(context.Background(), manifest, errors.New("build failed")); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(prompt.calls).To(Equal(1))
		loaded, err := lifecycle.Store.Load("run-1")
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(loaded.FailureArtifacts).NotTo(BeNil())
		Expect(loaded.FailureArtifacts.VerifiedAt).NotTo(BeZero())
		if _, err := lifecycle.Namespace.Inspect(context.Background(), "tenant"); err != nil {
			Fail("declined cleanup deleted namespace")
		}
	})

	It("suppresses prompt when artifact collection fails", func() {
		stateDir := GinkgoT().TempDir()
		client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespaceObject("tenant", "run-1"))
		prompt := &fakePrompt{approved: true}
		manifest := model.RunManifest{RunID: "run-1", Phase: model.PhaseFailed, Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
		lifecycle := Lifecycle{Store: evidence.NewManifestStore(stateDir), Namespace: cleanup.NamespaceService{Dynamic: client}, Collector: collector.Collector{StateDir: stateDir}, Prompt: prompt, Sources: []collector.Source{{Name: "applications", Path: "workload/applications.json", Collect: func(context.Context, string) error { return errors.New("api unavailable") }}}}
		err := lifecycle.Finalize(context.Background(), manifest, errors.New("build failed"))
		Expect(err).To(HaveOccurred())
		Expect(prompt.calls).To(Equal(0))
		if _, err := lifecycle.Namespace.Inspect(context.Background(), "tenant"); err != nil {
			Fail("collection failure deleted namespace")
		}
	})

	It("preserves result and namespace after declined successful cleanup", func() {
		stateDir := GinkgoT().TempDir()
		client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespaceObject("tenant", "run-1"))
		prompt := &fakePrompt{approved: false}
		manifest := model.RunManifest{RunID: "run-1", Phase: model.PhaseCompleted, Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
		store := evidence.NewManifestStore(stateDir)
		Expect(store.Create(&manifest)).Should(Succeed())
		lifecycle := Lifecycle{Store: store, Namespace: cleanup.NamespaceService{Dynamic: client}, Collector: collector.Collector{StateDir: stateDir}, Prompt: prompt, Sources: lifecycleTestSources()}
		if err := lifecycle.Finalize(context.Background(), manifest, nil); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(prompt.calls).To(Equal(1))
		report, err := collector.ReadReport(filepath.Join(store.RunDirFor(manifest), "collection-report.json"))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(report.RunID).To(Equal("run-1"))
		if _, err := lifecycle.Namespace.Inspect(context.Background(), "tenant"); err != nil {
			Fail("declined cleanup deleted namespace")
		}
	})

	It("suppresses successful cleanup when required artifact collection is incomplete", func() {
		stateDir := GinkgoT().TempDir()
		client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespaceObject("tenant", "run-1"))
		prompt := &fakePrompt{approved: false}
		manifest := model.RunManifest{RunID: "run-1", Phase: model.PhaseCompleted, Fixture: model.FixtureIdentity{TenantNamespace: "tenant"}}
		store := evidence.NewManifestStore(stateDir)
		Expect(store.Create(&manifest)).Should(Succeed())
		sources := lifecycleTestSources()
		sources[0].Collect = func(context.Context, string) error { return errors.New("api unavailable") }
		lifecycle := Lifecycle{Store: store, Namespace: cleanup.NamespaceService{Dynamic: client}, Collector: collector.Collector{StateDir: stateDir}, Prompt: prompt, Sources: sources}

		err := lifecycle.Finalize(context.Background(), manifest, nil)
		Expect(err).Should(MatchError(ContainSubstring("workload/applications.json")))
		Expect(prompt.calls).To(Equal(0))
		if _, err := lifecycle.Namespace.Inspect(context.Background(), "tenant"); err != nil {
			Fail("incomplete successful-run artifacts deleted namespace")
		}
	})
})

func lifecycleTestSources() []collector.Source {
	sources := collector.RequiredArtifactNames()
	result := make([]collector.Source, 0, len(sources)-2)
	for _, path := range sources[1 : len(sources)-1] {
		path := path
		result = append(result, collector.Source{Name: path, Path: path, Collect: func(_ context.Context, root string) error {
			if path == "workload/taskruns.json" {
				artifactPath := filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(artifactPath), 0o750); err != nil {
					return err
				}
				return os.WriteFile(artifactPath, []byte("{\"items\":[]}\n"), 0o640)
			}
			return writeLifecycleArtifact(filepath.Join(root, path))
		}})
	}
	return result
}

func namespaceObject(name, runID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": name, "labels": map[string]any{cleanup.ManagedByLabel: cleanup.ManagedByValue, cleanup.RunIDLabel: runID}}}}
}

func writeLifecycleArtifact(path string) error {
	if filepath.Ext(path) == "" {
		return os.MkdirAll(path, 0o750)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("{}\n"), 0o640)
}
