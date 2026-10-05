package cluster

import (
	"context"
	"strings"
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestBuildOutputInspectorRequiresBothLinuxPlatforms(t *testing.T) {
	client := buildOutputClient(t, "linux/amd64", "linux/arm64")
	got, err := (BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Platforms) != 2 {
		t.Fatalf("evidence = %#v", got)
	}
}

func TestBuildOutputInspectorRejectsMissingPlatform(t *testing.T) {
	client := buildOutputClient(t, "linux/amd64")
	_, err := (BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err == nil || !strings.Contains(err.Error(), "linux/arm64") {
		t.Fatalf("error = %v, want missing arm64", err)
	}
}

func TestBuildOutputInspectorRejectsFailedOrUnrelatedPipelineRun(t *testing.T) {
	client := buildOutputClient(t, "linux/amd64", "linux/arm64")
	failed := pipelineRunObject("build", "tenant", "uid-1", "False")
	client = fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), failed)
	_, err := (BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err == nil || !strings.Contains(err.Error(), "did not succeed") {
		t.Fatalf("error = %v, want failed PipelineRun", err)
	}

	client = buildOutputClient(t, "linux/amd64", "linux/arm64")
	_, err = (BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{{Namespace: "tenant", Name: "other", UID: "uid-other", Component: "web"}})
	if err == nil || !strings.Contains(err.Error(), "get PipelineRun") {
		t.Fatalf("error = %v, want unrelated PipelineRun failure", err)
	}
}

func buildOutputClient(t *testing.T, platforms ...string) *fake.FakeDynamicClient {
	t.Helper()
	pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
	taskRuns := make([]*unstructured.Unstructured, 0, len(platforms))
	for index, platform := range platforms {
		taskRuns = append(taskRuns, taskRunObject("build-task-"+string(rune('a'+index)), "tenant", "uid-1", platform))
	}
	objects := make([]runtime.Object, 0, len(taskRuns)+1)
	objects = append(objects, pipeline)
	for _, taskRun := range taskRuns {
		objects = append(objects, taskRun)
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), objects...)
}

func buildIdentity() model.PipelineRunIdentity {
	return model.PipelineRunIdentity{Namespace: "tenant", Name: "build", UID: "uid-1", Component: "web", SourceSHA: "sha-1"}
}

func pipelineRunObject(name, namespace, uid, status string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]any{
			"name": name, "namespace": namespace, "uid": uid,
		},
		"status": map[string]any{"conditions": []any{map[string]any{"type": "Succeeded", "status": status}}},
	}}
}

func taskRunObject(name, namespace, ownerUID, platform string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "TaskRun",
		"metadata": map[string]any{
			"name": name, "namespace": namespace,
			"ownerReferences": []any{map[string]any{"apiVersion": "tekton.dev/v1", "kind": "PipelineRun", "name": "build", "uid": ownerUID, "controller": true}},
		},
		"status": map[string]any{"conditions": []any{map[string]any{"type": "Succeeded", "status": "True"}}, "results": []any{
			map[string]any{"name": "PLATFORM", "value": platform},
			map[string]any{"name": "BUILD_OUTPUT", "value": "created:" + platform},
		}},
	}}
}

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}: "PipelineRunList",
		{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}:     "TaskRunList",
	}
}
