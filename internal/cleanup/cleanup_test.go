package cleanup

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

var _ = Describe("NamespaceService", func() {
	It("deletes only exact owned namespace", func() {
		client := cleanupClient(namespace("tenant", "run-1", true))
		service := NamespaceService{Dynamic: client}
		if err := service.Delete(context.Background(), "tenant", "run-1"); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(service.WaitDeleted(context.Background(), "tenant")).To(Succeed())
	})

	It("strips finalizers before delete", func() {
		appGVR := schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}
		client := cleanupClient(
			namespace("tenant", "run-1", true),
			&unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "appstudio.redhat.com/v1alpha1",
				"kind":       "Application",
				"metadata": map[string]any{
					"name": "app-1", "namespace": "tenant", "finalizers": []any{"appstudio.redhat.com/finalizer"},
				},
			}},
		)

		if err := (NamespaceService{Dynamic: client}).Delete(context.Background(), "tenant", "run-1"); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		updated, err := client.Resource(appGVR).Namespace("tenant").Get(context.Background(), "app-1", metav1.GetOptions{})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(updated.GetFinalizers()).To(BeEmpty())
	})

	It("refuses unowned namespace", func() {
		client := cleanupClient(namespace("tenant", "run-1", false))
		err := (NamespaceService{Dynamic: client}).Delete(context.Background(), "tenant", "run-1")
		Expect(err).To(MatchError(ContainSubstring("refusing")))
		if _, err := client.Resource(NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{}); err != nil {
			Fail("namespace was deleted")
		}
	})

	It("requires run ID for deletion", func() {
		client := cleanupClient(namespace("tenant", "run-1", true))
		err := (NamespaceService{Dynamic: client}).Delete(context.Background(), "tenant", "")
		Expect(err).To(HaveOccurred())
	})

	It("finds candidates using CLI names and ownership selector", func() {
		client := fake.NewSimpleDynamicClient(
			runtime.NewScheme(),
			namespace("mathwizz-test-github", "run-1", true),
			namespace("mathwizz-test-github-20261005b", "run-2", true),
			namespace("other-tenant", "run-3", true),
		)
		service := NamespaceService{
			Dynamic:    client,
			NameLister: staticNameLister{names: []string{"mathwizz-test-github", "mathwizz-test-github-20261005b"}},
		}

		candidates, err := service.FindCandidates(context.Background(), "mathwizz-test-github")
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(candidates).To(HaveLen(2))
		Expect(candidates[0].Name).To(Equal("mathwizz-test-github"))
		Expect(candidates[1].Name).To(Equal("mathwizz-test-github-20261005b"))
	})

	It("rejects unowned CLI name", func() {
		client := fake.NewSimpleDynamicClient(
			runtime.NewScheme(),
			namespace("mathwizz-test-github", "run-1", true),
			namespace("mathwizz-test-github-20261005b", "run-2", false),
		)
		service := NamespaceService{
			Dynamic:    client,
			NameLister: staticNameLister{names: []string{"mathwizz-test-github", "mathwizz-test-github-20261005b"}},
		}

		_, err := service.FindCandidates(context.Background(), "mathwizz-test-github")
		Expect(err).To(MatchError(ContainSubstring("without exact konflux-test ownership labels")))
	})

	It("uses CLI output for namespace names", func() {
		script := filepath.Join(GinkgoT().TempDir(), "oc")
		content := "#!/bin/sh\n[ \"$1 $2 $3 $4 $5\" = \"get namespaces -o name --no-headers\" ] || exit 1\nprintf '%s\\n' namespace/mathwizz-test-github namespace/mathwizz-test-github-20261005b\n"
		if err := os.WriteFile(script, []byte(content), 0700); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		names, err := (OCNamespaceNameLister{Command: script}).List(context.Background())
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		want := []string{"mathwizz-test-github", "mathwizz-test-github-20261005b"}
		Expect(names).To(Equal(want))
	})
})

type staticNameLister struct {
	names []string
}

func (l staticNameLister) List(context.Context) ([]string, error) {
	return append([]string(nil), l.names...), nil
}

func namespace(name, runID string, owned bool) *unstructured.Unstructured {
	labels := map[string]any{RunIDLabel: runID}
	if owned {
		labels[ManagedByLabel] = ManagedByValue
	}
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": name, "labels": labels}}}
}

func cleanupClient(objects ...runtime.Object) *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		NamespaceGVR: "NamespaceList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}:      "ApplicationList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"}:        "ComponentList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "imagerepositories"}: "ImageRepositoryList",
		{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "releases"}:          "ReleaseList",
		{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}:                      "PipelineRunList",
	}, objects...)
}
