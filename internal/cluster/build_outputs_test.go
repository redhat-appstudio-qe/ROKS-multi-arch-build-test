package cluster

import (
	"context"
	"strings"
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestBuildOutputInspectorRequiresBothLinuxPlatforms(t *testing.T) {
	client, kubernetesClient := buildOutputClient(t, "linux/amd64", "linux/arm64")
	got, err := (BuildOutputInspector{Dynamic: client, Kubernetes: kubernetesClient}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Platforms) != 2 {
		t.Fatalf("evidence = %#v", got)
	}
}

func TestBuildOutputInspectorRejectsMissingPlatform(t *testing.T) {
	client, kubernetesClient := buildOutputClient(t, "linux/amd64")
	_, err := (BuildOutputInspector{Dynamic: client, Kubernetes: kubernetesClient}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err == nil || !strings.Contains(err.Error(), "linux/arm64") {
		t.Fatalf("error = %v, want missing arm64", err)
	}
}

func TestBuildOutputInspectorUsesBuildahResultsAndPodNodeArchitecture(t *testing.T) {
	pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
	amd64TaskRun := buildahTaskRunObject("build-task-amd64", "tenant", "uid-1", "build-pod-amd64", "sha256:amd64")
	arm64TaskRun := buildahTaskRunObject("build-task-arm64", "tenant", "uid-1", "build-pod-arm64", "sha256:arm64")
	objects := []runtime.Object{pipeline, amd64TaskRun, arm64TaskRun}
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), objects...)
	kubernetesClient := kubernetesfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "build-pod-amd64", Namespace: "tenant"}, Spec: corev1.PodSpec{NodeName: "node-amd64"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "build-pod-arm64", Namespace: "tenant"}, Spec: corev1.PodSpec{NodeName: "node-arm64"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-amd64", Labels: map[string]string{"kubernetes.io/arch": "amd64"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-arm64", Labels: map[string]string{"kubernetes.io/arch": "arm64"}}},
	)

	got, err := (BuildOutputInspector{Dynamic: dynamicClient, Kubernetes: kubernetesClient}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Platforms) != 2 {
		t.Fatalf("evidence = %#v", got)
	}
	if got[0].CreatedOutputs["linux/amd64"] != "quay.io/example/image" || got[0].CreatedOutputs["linux/arm64"] != "quay.io/example/image" || got[0].OutputDigests["linux/amd64"] != "sha256:amd64" || got[0].OutputDigests["linux/arm64"] != "sha256:arm64" {
		t.Fatalf("created outputs = %#v digests = %#v", got[0].CreatedOutputs, got[0].OutputDigests)
	}
}

func TestBuildOutputInspectorRejectsIncompleteBuildahResults(t *testing.T) {
	pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
	amd64TaskRun := taskRunWithDeclaredPlatform("build-task-amd64", "tenant", "uid-1", "linux/amd64", "quay.io/example/amd64", "")
	arm64TaskRun := taskRunWithDeclaredPlatform("build-task-arm64", "tenant", "uid-1", "linux/arm64", "quay.io/example/arm64", "sha256:arm64")
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), pipeline, amd64TaskRun, arm64TaskRun)

	_, err := (BuildOutputInspector{Dynamic: dynamicClient, Kubernetes: kubernetesfake.NewSimpleClientset()}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err == nil || !strings.Contains(err.Error(), "IMAGE_DIGEST") {
		t.Fatalf("error = %v, want incomplete IMAGE_DIGEST evidence", err)
	}
}

func TestBuildOutputInspectorUsesAssignedNodeArchitecture(t *testing.T) {
	pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
	amd64TaskRun := taskRunWithDeclaredPlatform("build-task-amd64", "tenant", "uid-1", "linux/arm64", "quay.io/example/amd64", "sha256:amd64")
	arm64TaskRun := taskRunWithDeclaredPlatform("build-task-arm64", "tenant", "uid-1", "linux/amd64", "quay.io/example/arm64", "sha256:arm64")
	amd64TaskRun.Object["status"].(map[string]any)["podName"] = "build-pod-amd64"
	arm64TaskRun.Object["status"].(map[string]any)["podName"] = "build-pod-arm64"
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), pipeline, amd64TaskRun, arm64TaskRun)
	kubernetesClient := kubernetesfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "build-pod-amd64", Namespace: "tenant"}, Spec: corev1.PodSpec{NodeName: "node-amd64"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "build-pod-arm64", Namespace: "tenant"}, Spec: corev1.PodSpec{NodeName: "node-arm64"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-amd64", Labels: map[string]string{"kubernetes.io/arch": "amd64"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-arm64", Labels: map[string]string{"kubernetes.io/arch": "arm64"}}},
	)

	got, err := (BuildOutputInspector{Dynamic: dynamicClient, Kubernetes: kubernetesClient}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].CreatedOutputs["linux/amd64"] != "quay.io/example/amd64" || got[0].CreatedOutputs["linux/arm64"] != "quay.io/example/arm64" {
		t.Fatalf("created outputs = %#v", got[0].CreatedOutputs)
	}
}

func TestBuildOutputInspectorRequiresBuildahTaskIdentity(t *testing.T) {
	taskRun := taskRunObject("build-task", "tenant", "uid-1", "linux/amd64")
	taskRun.SetLabels(map[string]string{"tekton.dev/pipelineTask": "build-container", "tekton.dev/task": "other-task"})
	if isBuildTaskRun(taskRun) {
		t.Fatal("non-Buildah TaskRun was accepted as build output evidence")
	}
}

func TestBuildOutputInspectorRejectsFailedOrUnrelatedPipelineRun(t *testing.T) {
	client, kubernetesClient := buildOutputClient(t, "linux/amd64", "linux/arm64")
	failed := pipelineRunObject("build", "tenant", "uid-1", "False")
	client = fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), failed)
	_, err := (BuildOutputInspector{Dynamic: client, Kubernetes: kubernetesClient}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
	if err == nil || !strings.Contains(err.Error(), "did not succeed") {
		t.Fatalf("error = %v, want failed PipelineRun", err)
	}

	client, kubernetesClient = buildOutputClient(t, "linux/amd64", "linux/arm64")
	_, err = (BuildOutputInspector{Dynamic: client, Kubernetes: kubernetesClient}).Verify(context.Background(), []model.PipelineRunIdentity{{Namespace: "tenant", Name: "other", UID: "uid-other", Component: "web"}})
	if err == nil || !strings.Contains(err.Error(), "get PipelineRun") {
		t.Fatalf("error = %v, want unrelated PipelineRun failure", err)
	}
}

func buildOutputClient(t *testing.T, platforms ...string) (*fake.FakeDynamicClient, kubernetes.Interface) {
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
	kubeObjects := make([]runtime.Object, 0, len(taskRuns)*2)
	for index, taskRun := range taskRuns {
		platform := platforms[index]
		podName, _, _ := unstructured.NestedString(taskRun.Object, "status", "podName")
		nodeName := "node-" + strings.TrimPrefix(strings.ReplaceAll(platform, "/", "-"), "linux-")
		kubeObjects = append(kubeObjects,
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: "tenant"}, Spec: corev1.PodSpec{NodeName: nodeName}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: map[string]string{"kubernetes.io/arch": strings.TrimPrefix(strings.ReplaceAll(platform, "/", "-"), "linux-")}}},
		)
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), objects...), kubernetesfake.NewSimpleClientset(kubeObjects...)
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
			"labels":          map[string]any{"tekton.dev/pipelineTask": "build-container", "tekton.dev/task": "buildah-oci-ta"},
			"ownerReferences": []any{map[string]any{"apiVersion": "tekton.dev/v1", "kind": "PipelineRun", "name": "build", "uid": ownerUID, "controller": true}},
		},
		"status": map[string]any{"podName": "build-pod-" + strings.TrimPrefix(strings.ReplaceAll(platform, "/", "-"), "linux-"), "conditions": []any{map[string]any{"type": "Succeeded", "status": "True"}}, "results": []any{
			map[string]any{"name": "PLATFORM", "value": platform},
			map[string]any{"name": "IMAGE_URL", "value": "quay.io/example/image"},
			map[string]any{"name": "IMAGE_DIGEST", "value": "sha256:" + strings.ReplaceAll(platform, "/", "-")},
		}},
	}}
}

func buildahTaskRunObject(name, namespace, ownerUID, podName, digest string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "TaskRun",
		"metadata": map[string]any{
			"name": name, "namespace": namespace,
			"labels":          map[string]any{"tekton.dev/pipelineTask": "build-container", "tekton.dev/task": "buildah-oci-ta"},
			"ownerReferences": []any{map[string]any{"apiVersion": "tekton.dev/v1", "kind": "PipelineRun", "name": "build", "uid": ownerUID, "controller": true}},
		},
		"status": map[string]any{
			"podName":    podName,
			"conditions": []any{map[string]any{"type": "Succeeded", "status": "True"}},
			"results": []any{
				map[string]any{"name": "IMAGE_URL", "value": "quay.io/example/image"},
				map[string]any{"name": "IMAGE_DIGEST", "value": digest},
			},
		},
	}}
}

func taskRunWithDeclaredPlatform(name, namespace, ownerUID, platform, imageURL, digest string) *unstructured.Unstructured {
	taskRun := buildahTaskRunObject(name, namespace, ownerUID, "build-pod-"+strings.TrimPrefix(strings.ReplaceAll(platform, "/", "-"), "linux-"), digest)
	taskRun.Object["status"].(map[string]any)["results"] = []any{
		map[string]any{"name": "PLATFORM", "value": platform},
		map[string]any{"name": "IMAGE_URL", "value": imageURL},
		map[string]any{"name": "IMAGE_DIGEST", "value": digest},
	}
	if digest == "" {
		taskRun.Object["status"].(map[string]any)["results"] = []any{
			map[string]any{"name": "PLATFORM", "value": platform},
			map[string]any{"name": "IMAGE_URL", "value": imageURL},
		}
	}
	return taskRun
}

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}: "PipelineRunList",
		{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}:     "TaskRunList",
	}
}
