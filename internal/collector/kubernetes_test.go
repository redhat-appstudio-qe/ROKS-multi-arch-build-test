package collector

import (
	"context"
	"os"
	"path/filepath"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

var _ = ginkgo.Describe("Kubernetes sources", func() {
	ginkgo.It("writes one redacted YAML snapshot per TaskRun", func() {
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "tekton.dev/v1",
			"kind":       "TaskRun",
			"metadata": map[string]any{
				"name":      "build-run",
				"namespace": "tenant",
				"annotations": map[string]any{
					"secret-token": "secret-value",
				},
			},
			"spec": map[string]any{
				"params": []any{map[string]any{
					"name":  "PARAM_PLATFORM",
					"value": "",
				}},
			},
		}}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{collectorTaskRunGVR(): "TaskRunList"})
		if _, err := client.Resource(collectorTaskRunGVR()).Namespace("tenant").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		root := ginkgo.GinkgoT().TempDir()
		var source Source
		for _, candidate := range KubernetesSources(client, "tenant", "run-1") {
			if candidate.Name == "taskrun-yaml" {
				source = candidate
			}
		}
		Expect(source.Collect).NotTo(BeNil())
		Expect(source.Collect(context.Background(), root)).To(Succeed())

		data, err := os.ReadFile(filepath.Join(root, "workload", "taskruns", "tenant--build-run.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("name: PARAM_PLATFORM"))
		Expect(string(data)).To(ContainSubstring("value: \"\""))
		Expect(string(data)).To(ContainSubstring("name: build-run"))
		Expect(string(data)).To(ContainSubstring("secret-token: '[REDACTED]'"))
		Expect(string(data)).NotTo(ContainSubstring("secret-value"))
	})

	ginkgo.It("write redacted snapshots", func() {
		object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "pod", "namespace": "tenant", "password": "secret-value"}}}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Version: "v1", Resource: "pods"}: "PodList"})
		if _, err := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace("tenant").Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		root := ginkgo.GinkgoT().TempDir()
		for _, source := range KubernetesSources(client, "tenant", "run-1") {
			if source.Name == "pods" {
				if err := source.Collect(context.Background(), root); err != nil {
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}
		data, err := os.ReadFile(filepath.Join(root, "workload", "pods.json"))
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(string(data)).NotTo(BeEmpty())
	})

	ginkgo.It("uses run start as SinceTime", func() {
		startedAt := time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC)
		client := kubernetesfake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "build-pod", Namespace: "tenant"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "build"}}}})
		var gotOptions corev1.PodLogOptions
		source := newPodLogSource(client, "tenant", startedAt, func(_ context.Context, namespace, pod, container string, options corev1.PodLogOptions) ([]byte, error) {
			if namespace != "tenant" || pod != "build-pod" || container != "build" {
				ginkgo.Fail("unexpected log target: " + namespace + "/" + pod + "/" + container)
			}
			gotOptions = options
			return []byte("build output\n"), nil
		})
		root := ginkgo.GinkgoT().TempDir()
		if err := source.Collect(context.Background(), root); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(gotOptions.SinceTime).NotTo(BeNil())
		Expect(gotOptions.SinceTime.Time).To(Equal(startedAt))
		Expect(gotOptions.Timestamps).To(BeTrue())
		data, err := os.ReadFile(filepath.Join(root, "logs", "tenant", "build-pod", "build.log"))
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(string(data)).To(ContainSubstring("build output"))
	})
})

func collectorTaskRunGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}
}
