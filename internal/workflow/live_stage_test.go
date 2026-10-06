package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/cleanup"
	"github.com/redhat-appstudio/konflux-test/internal/cluster"
	"github.com/redhat-appstudio/konflux-test/internal/collector"
	"github.com/redhat-appstudio/konflux-test/internal/config"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

type confirmationStub struct {
	approved bool
	calls    int
}

func (s *confirmationStub) Confirm(context.Context, string) (bool, error) {
	s.calls++
	return s.approved, nil
}

type testNameLister struct {
	names []string
}

func (l testNameLister) List(context.Context) ([]string, error) {
	return append([]string(nil), l.names...), nil
}

var _ = Describe("LiveStage", func() {
	It("allows missing tenant namespace", func() {
		stage := &LiveStage{
			Request:   config.Request{TenantNamespace: "tenant"},
			Namespace: cleanup.NamespaceService{Dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), NameLister: testNameLister{names: nil}},
		}
		Expect(stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-1"})).To(Succeed())
	})

	It("deletes owned stale namespace after approval", func() {
		dynamic := namespaceDynamicClient(namespaceObject("tenant", "run-1"))
		prompt := &confirmationStub{approved: true}
		stage := &LiveStage{
			Request:   config.Request{TenantNamespace: "tenant"},
			Prompt:    prompt,
			Namespace: cleanup.NamespaceService{Dynamic: dynamic, PollInterval: time.Millisecond, NameLister: testNameLister{names: []string{"tenant"}}},
		}
		Expect(stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-2"})).To(Succeed())
		Expect(prompt.calls).To(Equal(1))
		_, err := dynamic.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stops when stale namespace is retained", func() {
		dynamic := namespaceDynamicClient(namespaceObject("tenant", "run-1"))
		prompt := &confirmationStub{}
		stage := &LiveStage{
			Request:   config.Request{TenantNamespace: "tenant"},
			Prompt:    prompt,
			Namespace: cleanup.NamespaceService{Dynamic: dynamic, NameLister: testNameLister{names: []string{"tenant"}}},
		}
		Expect(stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-2"})).To(HaveOccurred())
		Expect(prompt.calls).To(Equal(1))
	})

	It("rejects unowned namespace", func() {
		dynamic := namespaceDynamicClient(namespaceObject("tenant", "run-1"))
		object, err := dynamic.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		object.SetLabels(map[string]string{"app.konflux.org/run-id": "run-1"})
		_, err = dynamic.Resource(cleanup.NamespaceGVR).Update(context.Background(), object, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())
		stage := &LiveStage{
			Request:   config.Request{TenantNamespace: "tenant"},
			Prompt:    &confirmationStub{approved: true},
			Namespace: cleanup.NamespaceService{Dynamic: dynamic, NameLister: testNameLister{names: []string{"tenant"}}},
		}
		Expect(stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-2"})).To(HaveOccurred())
	})

	It("deletes owned suffixed namespaces", func() {
		dynamic := namespaceDynamicClient(
			namespaceObject("tenant", "run-1"),
			namespaceObject("tenant-20261005b", "run-2"),
		)
		prompt := &confirmationStub{approved: true}
		stage := &LiveStage{
			Request: config.Request{TenantNamespace: "tenant"},
			Prompt:  prompt,
			Namespace: cleanup.NamespaceService{
				Dynamic:      dynamic,
				PollInterval: time.Millisecond,
				NameLister:   testNameLister{names: []string{"tenant-20261005b", "tenant"}},
			},
		}

		Expect(stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-3"})).To(Succeed())
		Expect(prompt.calls).To(Equal(2))
	})

	It("restores recorded canonical repository from manifest", func() {
		got := fixtureFromManifest(model.FixtureIdentity{
			RepositoryOwner: "redhat-appstudio-qe",
			RepositoryName:  "dr_test_mathwizz",
			Repository:      "https://github.com/redhat-appstudio-qe/dr_test_mathwizz",
		})
		want := providers.FixtureRepository{Owner: "redhat-appstudio-qe", Name: "dr_test_mathwizz", URL: "https://github.com/redhat-appstudio-qe/dr_test_mathwizz"}
		Expect(got).To(Equal(want))
	})

	It("polls until pipeline runs succeed", func() {
		commits := []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "sha-1"}}
		calls := 0
		identities, err := waitForBuildMatches(context.Background(), time.Second, time.Second, time.Millisecond, "mathwizz-test", commits, func(context.Context) ([]unstructured.Unstructured, error) {
			calls++
			if calls == 1 {
				return nil, nil
			}
			return []unstructured.Unstructured{pipelineRun("run-1", "mathwizz-web-server", "sha-1", "uid-1", "True")}, nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(BeNumerically(">=", 2))
		Expect(identities).To(HaveLen(1))
		Expect(identities[0].Name).To(Equal("run-1"))
		Expect(identities[0].Succeeded).To(BeTrue())
	})

	It("rejects failed PipelineRun", func() {
		commits := []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "sha-1"}}
		_, err := waitForBuildMatches(context.Background(), time.Second, time.Second, time.Millisecond, "mathwizz-test", commits, func(context.Context) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{pipelineRun("run-1", "mathwizz-web-server", "sha-1", "uid-1", "False")}, nil
		})
		Expect(err).To(HaveOccurred())
	})

	It("fails with component, namespace, trigger SHA, and observed count when no PipelineRun appears", func() {
		commit := model.TriggerCommit{Component: "mathwizz-web-server", SHA: "trigger-sha", CreatedAt: time.Now().UTC().Add(-time.Second)}
		_, err := waitForBuildMatches(context.Background(), 10*time.Millisecond, time.Second, time.Millisecond, "mathwizz-test", []model.TriggerCommit{commit}, func(context.Context) ([]unstructured.Unstructured, error) {
			return nil, nil
		})
		Expect(err).To(MatchError(And(
			ContainSubstring("mathwizz-web-server"),
			ContainSubstring("mathwizz-test"),
			ContainSubstring("trigger-sha"),
			ContainSubstring("observed 0 PipelineRuns"),
		)))
	})

	It("accepts one post-trigger PipelineRun when source SHA differs", func() {
		triggeredAt := time.Now().UTC()
		object := pipelineRun("run-fallback", "mathwizz-web-server", "observed-sha", "uid-fallback", "True")
		object.SetCreationTimestamp(metav1.NewTime(triggeredAt.Add(time.Second)))
		identities, pending, err := matchBuildsWithObjects([]unstructured.Unstructured{object}, []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "trigger-sha", CreatedAt: triggeredAt}})
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(BeFalse())
		Expect(identities).To(HaveLen(1))
		Expect(identities[0].SourceSHA).To(Equal("observed-sha"))
	})

	It("rejects stale and ambiguous fallback PipelineRuns", func() {
		triggeredAt := time.Now().UTC()
		stale := pipelineRun("run-stale", "mathwizz-web-server", "stale-sha", "uid-stale", "True")
		stale.SetCreationTimestamp(metav1.NewTime(triggeredAt.Add(-time.Second)))
		_, pending, err := matchBuildsWithObjects([]unstructured.Unstructured{stale}, []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "trigger-sha", CreatedAt: triggeredAt}})
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(BeTrue())

		first := pipelineRun("run-fallback-1", "mathwizz-web-server", "observed-sha-1", "uid-fallback-1", "Unknown")
		first.SetCreationTimestamp(metav1.NewTime(triggeredAt.Add(time.Second)))
		second := pipelineRun("run-fallback-2", "mathwizz-web-server", "observed-sha-2", "uid-fallback-2", "Unknown")
		second.SetCreationTimestamp(metav1.NewTime(triggeredAt.Add(2 * time.Second)))
		_, _, err = matchBuildsWithObjects([]unstructured.Unstructured{first, second}, []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "trigger-sha", CreatedAt: triggeredAt}})
		Expect(err).To(MatchError(ContainSubstring("ambiguous")))
	})

	It("collects failed container logs", func() {
		pipeline := pipelineRun("run-1", "mathwizz-web-server", "sha-1", "uid-1", "False")
		pipeline.SetNamespace("tenant")
		taskRun := failedTaskRunForWorkflow()
		dynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"}: "ComponentList",
			{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}:               "PipelineRunList",
			{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}:                   "TaskRunList",
		}, &pipeline, taskRun)
		kube := kubernetesfake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "build-pod", Namespace: "tenant"}})
		stateDir := GinkgoT().TempDir()
		stage := &LiveStage{
			Request:              config.Request{StateDir: stateDir, Timeouts: config.Timeouts{Trigger: time.Second, Build: time.Second}},
			Clients:              &cluster.ClientSet{Dynamic: dynamic, Kubernetes: kube},
			FailedBuildCollector: &collector.FailedBuildCollector{Dynamic: dynamic, Kubernetes: kube},
		}
		manifest := &model.RunManifest{
			ArtifactDirectory: "run-1",
			Fixture:           model.FixtureIdentity{TenantNamespace: "tenant"},
			TriggerCommits:    []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "sha-1"}},
		}
		err := stage.VerifyBuilds(context.Background(), manifest)
		var pipelineRunError *PipelineRunFailedError
		Expect(errors.As(err, &pipelineRunError)).To(BeTrue())
		Expect(manifest.FailedBuildLogs).To(HaveLen(1))
		Expect(manifest.FailedBuildLogs[0].LogPath).NotTo(BeEmpty())
		_, err = os.Stat(manifest.FailedBuildLogs[0].LogPath)
		Expect(err).NotTo(HaveOccurred())
	})

	It("counts baseline PipelineRuns", func() {
		objects := []unstructured.Unstructured{
			pipelineRun("existing-1", "mathwizz-web-server", "old-sha", "uid-old-1", "True"),
			pipelineRun("existing-2", "mathwizz-web-server", "old-sha-2", "uid-old-2", "False"),
			pipelineRun("existing-3", "mathwizz-web-server", "old-sha-3", "uid-old-3", "Unknown"),
		}
		succeeded, total := countBaselinePRs(objects)
		Expect(succeeded).To(Equal(1))
		Expect(total).To(Equal(3))

		succeeded, total = countBaselinePRs(nil)
		Expect(succeeded).To(BeZero())
		Expect(total).To(BeZero())
	})

	It("detects build overshoot", func() {
		commits := []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "sha-1"}}
		_, err := waitForBuildMatches(context.Background(), time.Second, time.Second, time.Millisecond, "mathwizz-test", commits, func(context.Context) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{
				pipelineRun("run-1", "mathwizz-web-server", "sha-1", "uid-1", "True"),
				pipelineRun("run-2", "mathwizz-web-server", "sha-1", "uid-2", "True"),
			}, nil
		})
		Expect(err).To(MatchError(ContainSubstring("overshoot")))
	})

	It("polls PaC before triggering commits", func() {
		components := []string{"mathwizz-web-server", "mathwizz-history-worker", "mathwizz-frontend"}
		calls := 0
		err := waitForPaCEnabled(context.Background(), time.Second, time.Millisecond, components, func(context.Context) ([]unstructured.Unstructured, error) {
			calls++
			state := "pending"
			if calls > 1 {
				state = "enabled"
			}
			objects := make([]unstructured.Unstructured, 0, len(components))
			for _, component := range components {
				objects = append(objects, componentWithPaCStatus(component, state))
			}
			return objects, nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(BeNumerically(">=", 2))
	})

	It("detects repository onboarding failure in PaC status", func() {
		object := unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{
				"name": "mathwizz-web-server",
				"annotations": map[string]any{
					"build.appstudio.openshift.io/status": `{"pac":{"state":"error","error-message":"repository is restricted"}}`,
				},
			},
		}}
		Expect(paCStatusError(&object)).To(HaveOccurred())
	})
})

func failedTaskRunForWorkflow() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1", "kind": "TaskRun",
		"metadata": map[string]any{
			"name": "build-task", "namespace": "tenant",
			"labels": map[string]any{"tekton.dev/pipelineRun": "run-1", "tekton.dev/pipelineTask": "build-container"},
		},
		"status": map[string]any{
			"podName":    "build-pod",
			"conditions": []any{map[string]any{"type": "Succeeded", "status": "False", "message": "step failed"}},
			"steps":      []any{map[string]any{"name": "build", "container": "step-build", "terminated": map[string]any{"exitCode": int64(1)}}},
		},
	}}
}

func pipelineRun(name, component, sha, uid, status string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "mathwizz-test",
			"uid":       uid,
			"labels": map[string]any{
				"appstudio.openshift.io/component": component,
				"pipelinesascode.tekton.dev/sha":   sha,
			},
		},
		"status": map[string]any{
			"conditions": []any{map[string]any{
				"type":               "Succeeded",
				"status":             status,
				"lastTransitionTime": time.Now().UTC().Format(time.RFC3339),
			}},
		},
	}}
}

func componentWithPaCStatus(name, state string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": name,
			"annotations": map[string]any{
				"build.appstudio.openshift.io/status": fmt.Sprintf(`{"pac":{"state":%q}}`, state),
			},
		},
	}}
}

func namespaceObject(name, runID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": name,
			"labels": map[string]any{
				cleanup.ManagedByLabel: cleanup.ManagedByValue,
				cleanup.RunIDLabel:     runID,
			},
		},
	}}
}

func namespaceDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		cleanup.NamespaceGVR: "NamespaceList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}:      "ApplicationList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"}:        "ComponentList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "imagerepositories"}: "ImageRepositoryList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "releases"}:          "ReleaseList",
		{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}:                      "PipelineRunList",
	}, objects...)
}
