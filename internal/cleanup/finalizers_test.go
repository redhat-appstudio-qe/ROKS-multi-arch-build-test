package cleanup_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/cleanup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

var _ = Describe("StripFinalizers", func() {
	var gvr schema.GroupVersionResource

	BeforeEach(func() {
		gvr = schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}
	})

	It("removes all finalizers from resources", func() {
		object := resource("Application", "app-1", []any{"appstudio.redhat.com/finalizer", "another-finalizer"})
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "ApplicationList"}, object)

		count, err := cleanup.StripFinalizers(context.Background(), client, "tenant", gvr)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))

		updated, err := client.Resource(gvr).Namespace("tenant").Get(context.Background(), "app-1", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.GetFinalizers()).To(BeEmpty())
	})

	It("skips resources without finalizers", func() {
		object := resource("Application", "app-1", nil)
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "ApplicationList"}, object)

		count, err := cleanup.StripFinalizers(context.Background(), client, "tenant", gvr)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("handles an empty namespace", func() {
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "ApplicationList"})

		count, err := cleanup.StripFinalizers(context.Background(), client, "empty-ns", gvr)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(0))
	})
})

var _ = Describe("StripAllFinalizers", func() {
	It("strips finalizers from multiple resource types", func() {
		appGVR := schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}
		compGVR := schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"}
		prGVR := schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}
		pipelineRun := resource("PipelineRun", "pr-1", []any{"f3"})
		pipelineRun.Object["apiVersion"] = "tekton.dev/v1"
		client := fake.NewSimpleDynamicClientWithCustomListKinds(
			runtime.NewScheme(),
			map[schema.GroupVersionResource]string{
				appGVR: "ApplicationList", compGVR: "ComponentList", prGVR: "PipelineRunList",
				{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "imagerepositories"}: "ImageRepositoryList",
				{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "releases"}:          "ReleaseList",
			},
			resource("Application", "app-1", []any{"f1"}),
			resource("Component", "comp-1", []any{"f2"}),
			pipelineRun,
		)

		count, err := cleanup.StripAllFinalizers(context.Background(), client, "tenant")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(3))
	})
})

func resource(kind, name string, finalizers []any) *unstructured.Unstructured {
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "appstudio.redhat.com/v1alpha1",
		"kind":       kind,
		"metadata": map[string]any{
			"name": name, "namespace": "tenant",
		},
	}}
	if finalizers != nil {
		object.Object["metadata"].(map[string]any)["finalizers"] = finalizers
	}
	return object
}
