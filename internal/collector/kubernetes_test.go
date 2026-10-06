package collector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesSourcesWriteRedactedSnapshots(t *testing.T) {
	object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "pod", "namespace": "tenant", "password": "secret-value"}}}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Version: "v1", Resource: "pods"}: "PodList"})
	if _, err := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace("tenant").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, source := range KubernetesSources(client, "tenant", "run-1") {
		if source.Name == "pods" {
			if err := source.Collect(context.Background(), root); err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "workload", "pods.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("empty snapshot")
	}
}

func TestPodLogSourceUsesRunStartAsSinceTime(t *testing.T) {
	startedAt := time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC)
	client := kubernetesfake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "build-pod", Namespace: "tenant"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "build"}}}})
	var gotOptions corev1.PodLogOptions
	source := newPodLogSource(client, "tenant", startedAt, func(_ context.Context, namespace, pod, container string, options corev1.PodLogOptions) ([]byte, error) {
		if namespace != "tenant" || pod != "build-pod" || container != "build" {
			t.Fatalf("log target = %s/%s/%s", namespace, pod, container)
		}
		gotOptions = options
		return []byte("build output\n"), nil
	})
	root := t.TempDir()
	if err := source.Collect(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if gotOptions.SinceTime == nil || !gotOptions.SinceTime.Time.Equal(startedAt) || !gotOptions.Timestamps {
		t.Fatalf("pod log options = %#v", gotOptions)
	}
	data, err := os.ReadFile(filepath.Join(root, "logs", "tenant", "build-pod", "build.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "build output") {
		t.Fatalf("log data = %q", data)
	}
}
