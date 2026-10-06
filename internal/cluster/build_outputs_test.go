package cluster_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/cluster"
	"github.com/redhat-appstudio/konflux-test/internal/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

var _ = Describe("BuildOutputInspector", func() {
	It("verifies both linux platforms", func() {
		inspector := cluster.BuildOutputInspector{Dynamic: buildOutputClientWithPlatformLabels("linux/amd64", "linux/arm64")}
		evidence, err := inspector.Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
		Expect(err).NotTo(HaveOccurred())
		Expect(evidence).To(HaveLen(1))
		Expect(evidence[0].Platforms).To(Equal([]string{"linux/amd64", "linux/arm64"}))
	})

	It("rejects missing arm64 platform", func() {
		inspector := cluster.BuildOutputInspector{Dynamic: buildOutputClientWithPlatformLabels("linux/amd64")}
		_, err := inspector.Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("linux/arm64"))
	})

	It("rejects missing platform with a pipeline misconfiguration error", func() {
		pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
		taskRun := buildahTaskRunWithNoPlatform("build-task", "tenant", "uid-1", "quay.io/example/image", "sha256:abc")
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), pipeline, taskRun)

		_, err := (cluster.BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("pipeline misconfiguration"))
	})

	It("uses declared TaskRun platform results", func() {
		pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
		amd64 := taskRunWithDeclaredPlatform("build-task-amd64", "tenant", "uid-1", "linux/amd64", "quay.io/example/amd64", "sha256:amd64")
		arm64 := taskRunWithDeclaredPlatform("build-task-arm64", "tenant", "uid-1", "linux/arm64", "quay.io/example/arm64", "sha256:arm64")
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), pipeline, amd64, arm64)

		evidence, err := (cluster.BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
		Expect(err).NotTo(HaveOccurred())
		Expect(evidence).To(HaveLen(1))
		Expect(evidence[0].CreatedOutputs["linux/amd64"]).To(Equal("quay.io/example/amd64"))
		Expect(evidence[0].CreatedOutputs["linux/arm64"]).To(Equal("quay.io/example/arm64"))
	})

	It("rejects incomplete Buildah results", func() {
		pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
		amd64 := taskRunWithDeclaredPlatform("build-task-amd64", "tenant", "uid-1", "linux/amd64", "quay.io/example/amd64", "")
		arm64 := taskRunWithDeclaredPlatform("build-task-arm64", "tenant", "uid-1", "linux/arm64", "quay.io/example/arm64", "sha256:arm64")
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), pipeline, amd64, arm64)

		_, err := (cluster.BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("IMAGE_DIGEST"))
	})

	It("requires Buildah task identity", func() {
		taskRun := taskRunObject("build-task", "tenant", "uid-1", "linux/amd64")
		taskRun.SetLabels(map[string]string{"tekton.dev/pipelineTask": "build-container", "tekton.dev/task": "other-task"})
		Expect(cluster.IsBuildTaskRun(taskRun)).To(BeFalse())
	})

	It("rejects failed PipelineRuns", func() {
		failed := pipelineRunObject("build", "tenant", "uid-1", "False")
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), failed)
		_, err := (cluster.BuildOutputInspector{Dynamic: client}).Verify(context.Background(), []model.PipelineRunIdentity{buildIdentity()})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("did not succeed"))
	})
})

func buildOutputClientWithPlatformLabels(platforms ...string) *fake.FakeDynamicClient {
	pipeline := pipelineRunObject("build", "tenant", "uid-1", "True")
	objects := []runtime.Object{pipeline}
	for index, platform := range platforms {
		objects = append(objects, taskRunObject("build-task-"+string(rune('a'+index)), "tenant", "uid-1", platform))
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds(), objects...)
}

func buildIdentity() model.PipelineRunIdentity {
	return model.PipelineRunIdentity{Namespace: "tenant", Name: "build", UID: "uid-1", Component: "web", SourceSHA: "sha-1"}
}

func pipelineRunObject(name, namespace, uid, status string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1", "kind": "PipelineRun",
		"metadata": map[string]any{"name": name, "namespace": namespace, "uid": uid},
		"status":   map[string]any{"conditions": []any{map[string]any{"type": "Succeeded", "status": status}}},
	}}
}

func taskRunObject(name, namespace, ownerUID, platform string) *unstructured.Unstructured {
	return taskRunWithDeclaredPlatform(name, namespace, ownerUID, platform, "quay.io/example/image", "sha256:"+platform)
}

func taskRunWithDeclaredPlatform(name, namespace, ownerUID, platform, imageURL, digest string) *unstructured.Unstructured {
	results := []any{map[string]any{"name": "PLATFORM", "value": platform}, map[string]any{"name": "IMAGE_URL", "value": imageURL}}
	if digest != "" {
		results = append(results, map[string]any{"name": "IMAGE_DIGEST", "value": digest})
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1", "kind": "TaskRun",
		"metadata": map[string]any{
			"name": name, "namespace": namespace,
			"labels":          map[string]any{"tekton.dev/pipelineTask": "build-container", "tekton.dev/task": "buildah-oci-ta"},
			"ownerReferences": []any{map[string]any{"apiVersion": "tekton.dev/v1", "kind": "PipelineRun", "name": "build", "uid": ownerUID, "controller": true}},
		},
		"status": map[string]any{"conditions": []any{map[string]any{"type": "Succeeded", "status": "True"}}, "results": results},
	}}
}

func buildahTaskRunWithNoPlatform(name, namespace, ownerUID, imageURL, digest string) *unstructured.Unstructured {
	taskRun := taskRunWithDeclaredPlatform(name, namespace, ownerUID, "", imageURL, digest)
	results := taskRun.Object["status"].(map[string]any)["results"].([]any)
	taskRun.Object["status"].(map[string]any)["results"] = results[1:]
	return taskRun
}

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}: "PipelineRunList",
		{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}:     "TaskRunList",
	}
}
