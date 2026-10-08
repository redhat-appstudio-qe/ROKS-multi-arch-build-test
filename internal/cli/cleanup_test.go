package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/konflux-test/internal/cleanup"
	"github.com/redhat-appstudio/konflux-test/internal/collector"
	"github.com/redhat-appstudio/konflux-test/internal/config"
	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

var _ = Describe("cleanupRun", func() {
	It("deletes the manifest namespace only after confirming exact ownership", func() {
		store := cleanupTestStore()
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		Expect(cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)).Should(Succeed())
		Expect(prompt.calls).Should(Equal(1))
		Expect(prompt.message).Should(ContainSubstring("tenant-run-1"))
		Expect(prompt.message).Should(ContainSubstring("run-1"))
		_, err := client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	})

	It("refuses a namespace with a different run ID before prompting", func() {
		store := cleanupTestStore()
		client := cleanupTestClient("another-run", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		err := cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)
		Expect(err).Should(MatchError(ContainSubstring("exact konflux-test ownership")))
		Expect(prompt.calls).Should(BeZero())
		_, err = client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("requires an explicit confirmation prompt", func() {
		store := cleanupTestStore()
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)

		err := cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, nil)
		Expect(err).Should(MatchError(ContainSubstring("explicit confirmation")))
		_, err = client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("retains the namespace when confirmation is declined", func() {
		store := cleanupTestStore()
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{}

		Expect(cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)).Should(Succeed())
		_, err := client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("refuses a run created for a different cluster", func() {
		store := cleanupTestStore()
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}
		request := cleanupTestRequest("run-1", store.Root)
		request.ClusterServer = "https://other.example"

		err := cleanupRun(context.Background(), request, store, cleanup.NamespaceService{Dynamic: client}, prompt)
		Expect(err).Should(MatchError(ContainSubstring("belongs to cluster")))
		Expect(prompt.calls).Should(BeZero())
	})

	It("refuses cleanup when a required saved artifact is missing", func() {
		store := cleanupTestStore()
		manifest, err := store.Load("run-1")
		Expect(err).ShouldNot(HaveOccurred())
		missingPath := filepath.Join(store.RunDirFor(manifest), "workload", "pods.json")
		Expect(os.Remove(missingPath)).Should(Succeed())
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		err = cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)
		Expect(err).Should(MatchError(ContainSubstring("workload/pods.json")))
		Expect(prompt.calls).Should(BeZero())
		_, err = client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("refuses cleanup when the saved collection report marks a required artifact incomplete", func() {
		store := cleanupTestStore()
		manifest, err := store.Load("run-1")
		Expect(err).ShouldNot(HaveOccurred())
		reportPath := filepath.Join(store.RunDirFor(manifest), "collection-report.json")
		report, err := collector.ReadReport(reportPath)
		Expect(err).ShouldNot(HaveOccurred())
		for index := range report.Attempts {
			if report.Attempts[index].Path == "workload/components.json" {
				report.Attempts[index].Status = "error"
				report.Attempts[index].Error = "component snapshot failed"
			}
		}
		data, err := json.Marshal(report)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(os.WriteFile(reportPath, data, 0o640)).Should(Succeed())
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		err = cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)
		Expect(err).Should(MatchError(ContainSubstring("workload/components.json")))
		Expect(prompt.calls).Should(BeZero())
		_, err = client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("refuses cleanup when a listed TaskRun YAML snapshot is missing", func() {
		store := cleanupTestStore()
		manifest, err := store.Load("run-1")
		Expect(err).ShouldNot(HaveOccurred())
		taskRunsPath := filepath.Join(store.RunDirFor(manifest), "workload", "taskruns.json")
		Expect(os.WriteFile(taskRunsPath, []byte("{\"items\":[{\"metadata\":{\"name\":\"build-task\"}}]}\n"), 0o640)).Should(Succeed())
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		err = cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)
		Expect(err).Should(MatchError(ContainSubstring("workload/taskruns/tenant-run-1--build-task.yaml")))
		Expect(prompt.calls).Should(BeZero())
		_, err = client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("refuses cleanup when a saved TaskRun snapshot has invalid identity", func() {
		store := cleanupTestStore()
		manifest, err := store.Load("run-1")
		Expect(err).ShouldNot(HaveOccurred())
		runRoot := store.RunDirFor(manifest)
		taskRunsPath := filepath.Join(runRoot, "workload", "taskruns.json")
		Expect(os.WriteFile(taskRunsPath, []byte("{\"items\":[{\"metadata\":{\"name\":\"build-task\"}}]}\n"), 0o640)).Should(Succeed())
		snapshotPath := filepath.Join(runRoot, "workload", "taskruns", "tenant-run-1--build-task.yaml")
		Expect(os.WriteFile(snapshotPath, []byte("kind: TaskRun\nmetadata:\n  name: another-task\n  namespace: tenant-run-1\n"), 0o640)).Should(Succeed())
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		err = cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)
		Expect(err).Should(MatchError(ContainSubstring("TaskRun snapshot")))
		Expect(prompt.calls).Should(BeZero())
		_, err = client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("allows cleanup after verifying retained failed-run artifacts", func() {
		store := cleanupTestFailedStore()
		manifest, err := store.Load("run-1")
		Expect(err).ShouldNot(HaveOccurred())
		report, err := collector.ReadReport(filepath.Join(store.RunDirFor(manifest), "collection-report.json"))
		Expect(err).ShouldNot(HaveOccurred())
		failedBuildsVerified := false
		for _, attempt := range report.Attempts {
			if attempt.Path == "failed-builds/" && attempt.Status == "success" {
				failedBuildsVerified = true
			}
		}
		Expect(failedBuildsVerified).Should(BeTrue())
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		Expect(cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)).Should(Succeed())
		Expect(prompt.calls).Should(Equal(1))
	})

	It("refuses cleanup when a recorded failed-build log is missing", func() {
		store := cleanupTestFailedStore()
		manifest, err := store.Load("run-1")
		Expect(err).ShouldNot(HaveOccurred())
		logPath := manifest.FailedBuildLogs[0].LogPath
		Expect(os.Remove(logPath)).Should(Succeed())
		client := cleanupTestClient("run-1", cleanup.ManagedByValue)
		prompt := &cleanupTestPrompt{approved: true}

		err = cleanupRun(context.Background(), cleanupTestRequest("run-1", store.Root), store, cleanup.NamespaceService{Dynamic: client}, prompt)
		Expect(err).Should(MatchError(ContainSubstring("failed-builds/build.log")))
		Expect(prompt.calls).Should(BeZero())
		_, err = client.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant-run-1", metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})
})

type cleanupTestPrompt struct {
	approved bool
	calls    int
	message  string
}

func (p *cleanupTestPrompt) Confirm(_ context.Context, message string) (bool, error) {
	p.calls++
	p.message = message
	return p.approved, nil
}

func cleanupTestRequest(runID, stateDir string) config.Request {
	return config.Request{Command: config.CommandCleanup, RunID: runID, ClusterServer: "https://api.example", StateDir: stateDir}
}

func cleanupTestStore() evidence.ManifestStore {
	store := evidence.NewManifestStore(GinkgoT().TempDir())
	manifest := model.RunManifest{
		RunID:               "run-1",
		Provider:            config.ProviderGitHub,
		Phase:               model.PhaseCompleted,
		TargetClusterServer: "https://api.example/",
		Fixture:             model.FixtureIdentity{TenantNamespace: "tenant-run-1"},
	}
	Expect(store.Create(&manifest)).Should(Succeed())
	_, err := (collector.Collector{StateDir: store.Root}).CollectAndVerify(context.Background(), manifest.RunID, manifest, lifecycleTestSources())
	Expect(err).ShouldNot(HaveOccurred())
	return store
}

func cleanupTestFailedStore() evidence.ManifestStore {
	store := evidence.NewManifestStore(GinkgoT().TempDir())
	manifest := model.RunManifest{
		RunID:               "run-1",
		ArtifactDirectory:   "run-1",
		Provider:            config.ProviderGitHub,
		Phase:               model.PhaseFailed,
		TargetClusterServer: "https://api.example/",
		Fixture:             model.FixtureIdentity{TenantNamespace: "tenant-run-1"},
		FailedBuildLogs:     []model.FailedBuildLog{{TaskRunName: "build-task", LogPath: filepath.Join(store.Root, "run-1", "failed-builds", "build.log")}},
	}
	Expect(store.Create(&manifest)).Should(Succeed())
	Expect(os.MkdirAll(filepath.Join(store.RunDirFor(manifest), "failed-builds"), 0o750)).Should(Succeed())
	Expect(os.WriteFile(manifest.FailedBuildLogs[0].LogPath, []byte("failure log"), 0o640)).Should(Succeed())
	_, err := (collector.Collector{StateDir: store.Root}).CollectAndVerify(context.Background(), manifest.RunID, manifest, lifecycleTestSources())
	Expect(err).ShouldNot(HaveOccurred())
	return store
}

func cleanupTestClient(runID, managedBy string) *fake.FakeDynamicClient {
	namespace := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": "tenant-run-1",
			"labels": map[string]any{
				cleanup.ManagedByLabel: managedBy,
				cleanup.RunIDLabel:     runID,
			},
		},
	}}
	listKinds := map[schema.GroupVersionResource]string{
		cleanup.NamespaceGVR: "NamespaceList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}:      "ApplicationList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"}:        "ComponentList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "imagerepositories"}: "ImageRepositoryList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "releases"}:          "ReleaseList",
		{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}:                      "PipelineRunList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, namespace)
}
