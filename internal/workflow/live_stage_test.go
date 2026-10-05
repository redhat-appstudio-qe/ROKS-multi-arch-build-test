package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/cleanup"
	"github.com/redhat-appstudio/konflux-test/internal/config"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type confirmationStub struct {
	approved bool
	calls    int
}

func (s *confirmationStub) Confirm(context.Context, string) (bool, error) {
	s.calls++
	return s.approved, nil
}

func TestEnsureTenantNamespaceAvailableAllowsMissingNamespace(t *testing.T) {
	stage := &LiveStage{
		Request:   config.Request{TenantNamespace: "tenant"},
		Namespace: cleanup.NamespaceService{Dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), NameLister: testNameLister{names: nil}},
	}
	if err := stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureTenantNamespaceAvailableDeletesOwnedStaleNamespaceAfterApproval(t *testing.T) {
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), namespaceObject("tenant", "run-1"))
	prompt := &confirmationStub{approved: true}
	stage := &LiveStage{
		Request:   config.Request{TenantNamespace: "tenant"},
		Prompt:    prompt,
		Namespace: cleanup.NamespaceService{Dynamic: dynamic, PollInterval: time.Millisecond, NameLister: testNameLister{names: []string{"tenant"}}},
	}
	if err := stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-2"}); err != nil {
		t.Fatal(err)
	}
	if prompt.calls != 1 {
		t.Fatalf("confirmation calls = %d, want 1", prompt.calls)
	}
	if _, err := dynamic.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("namespace remains after approved deletion: %v", err)
	}
}

func TestEnsureTenantNamespaceAvailableStopsWhenStaleNamespaceIsRetained(t *testing.T) {
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), namespaceObject("tenant", "run-1"))
	prompt := &confirmationStub{}
	stage := &LiveStage{
		Request:   config.Request{TenantNamespace: "tenant"},
		Prompt:    prompt,
		Namespace: cleanup.NamespaceService{Dynamic: dynamic, NameLister: testNameLister{names: []string{"tenant"}}},
	}
	if err := stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-2"}); err == nil {
		t.Fatal("expected retained namespace to stop preflight")
	}
	if prompt.calls != 1 {
		t.Fatalf("confirmation calls = %d, want 1", prompt.calls)
	}
}

func TestEnsureTenantNamespaceAvailableRejectsUnownedNamespace(t *testing.T) {
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), namespaceObject("tenant", "run-1"))
	object, err := dynamic.Resource(cleanup.NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	object.SetLabels(map[string]string{"app.konflux.org/run-id": "run-1"})
	if _, err := dynamic.Resource(cleanup.NamespaceGVR).Update(context.Background(), object, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	stage := &LiveStage{
		Request:   config.Request{TenantNamespace: "tenant"},
		Prompt:    &confirmationStub{approved: true},
		Namespace: cleanup.NamespaceService{Dynamic: dynamic, NameLister: testNameLister{names: []string{"tenant"}}},
	}
	if err := stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-2"}); err == nil {
		t.Fatal("expected unowned namespace rejection")
	}
}

func TestEnsureTenantNamespaceAvailableDeletesOwnedSuffixedNamespaces(t *testing.T) {
	dynamic := dynamicfake.NewSimpleDynamicClient(
		runtime.NewScheme(),
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

	if err := stage.ensureTenantNamespaceAvailable(context.Background(), &model.RunManifest{RunID: "run-3"}); err != nil {
		t.Fatal(err)
	}
	if prompt.calls != 2 {
		t.Fatalf("confirmation calls = %d, want 2", prompt.calls)
	}
}

type testNameLister struct {
	names []string
}

func (l testNameLister) List(context.Context) ([]string, error) {
	return append([]string(nil), l.names...), nil
}

func TestFixtureFromManifestRestoresRecordedCanonicalRepository(t *testing.T) {
	got := fixtureFromManifest(model.FixtureIdentity{
		RepositoryOwner: "redhat-appstudio-qe",
		RepositoryName:  "dr_test_mathwizz",
		Repository:      "https://github.com/redhat-appstudio-qe/dr_test_mathwizz",
	})
	want := providers.FixtureRepository{Owner: "redhat-appstudio-qe", Name: "dr_test_mathwizz", URL: "https://github.com/redhat-appstudio-qe/dr_test_mathwizz"}
	if got != want {
		t.Fatalf("fixture = %#v, want %#v", got, want)
	}
}

func TestWaitForBuildMatchesPollsUntilPipelineRunsSucceed(t *testing.T) {
	commits := []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "sha-1"}}
	calls := 0
	identities, err := waitForBuildMatches(context.Background(), time.Second, time.Millisecond, commits, func(context.Context) ([]unstructured.Unstructured, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		return []unstructured.Unstructured{pipelineRun("run-1", "mathwizz-web-server", "sha-1", "uid-1", "True")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 || len(identities) != 1 || identities[0].Name != "run-1" || !identities[0].Succeeded {
		t.Fatalf("calls=%d identities=%#v", calls, identities)
	}
}

func TestWaitForBuildMatchesRejectsFailedPipelineRun(t *testing.T) {
	commits := []model.TriggerCommit{{Component: "mathwizz-web-server", SHA: "sha-1"}}
	_, err := waitForBuildMatches(context.Background(), time.Second, time.Millisecond, commits, func(context.Context) ([]unstructured.Unstructured, error) {
		return []unstructured.Unstructured{pipelineRun("run-1", "mathwizz-web-server", "sha-1", "uid-1", "False")}, nil
	})
	if err == nil {
		t.Fatal("expected failed PipelineRun error")
	}
}

func TestWaitForPaCEnabledPollsBeforeTriggeringCommits(t *testing.T) {
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
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("calls = %d, want at least 2", calls)
	}
}

func TestPaCStatusErrorDetectsRepositoryOnboardingFailure(t *testing.T) {
	object := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "mathwizz-web-server",
			"annotations": map[string]any{
				"build.appstudio.openshift.io/status": `{"pac":{"state":"error","error-message":"repository is restricted"}}`,
			},
		},
	}}
	if err := paCStatusError(&object); err == nil {
		t.Fatal("expected PaC status error")
	}
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
