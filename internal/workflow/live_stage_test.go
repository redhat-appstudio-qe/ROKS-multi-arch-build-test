package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
