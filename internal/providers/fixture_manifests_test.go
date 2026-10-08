package providers

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

const remoteBuildahBundle = "quay.io/konflux-ci/tekton-catalog/task-buildah-remote-oci-ta:0.12.3@sha256:88b971c800b3e71903ddad83da884d092e9beda8b51deaf980f6525e45ed043f"

var _ = Describe("multi-architecture fixture PipelineRuns", func() {
	When("the sibling fixture checkouts are present", func() {
		It("builds both platforms and combines their images for each provider component", func() {
			workspaceRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
			Expect(err).ShouldNot(HaveOccurred())

			fixtureRepos := []string{"DR_test_MathWizz", "dr_test_mathwizz_gl"}
			for _, fixtureRepo := range fixtureRepos {
				fixtureRoot := filepath.Join(workspaceRoot, fixtureRepo)
				_, err := os.Stat(fixtureRoot)
				if os.IsNotExist(err) {
					Skip(fmt.Sprintf("sibling fixture checkout %q is absent", fixtureRoot))
				}
				Expect(err).ShouldNot(HaveOccurred(), "checking sibling fixture checkout %q", fixtureRoot)
			}

			manifests := []string{
				"konflux-test-mathwizz-web-server-push.yaml",
				"konflux-test-mathwizz-history-worker-push.yaml",
				"konflux-test-mathwizz-frontend-push.yaml",
			}
			for _, fixtureRepo := range fixtureRepos {
				for _, manifest := range manifests {
					manifestPath := filepath.Join(workspaceRoot, fixtureRepo, ".tekton", manifest)
					assertMultiArchitecturePipelineRun(manifestPath)
				}
			}
		})
	})
})

func assertMultiArchitecturePipelineRun(manifestPath string) {
	contents, err := os.ReadFile(manifestPath)
	Expect(err).ShouldNot(HaveOccurred(), "reading PipelineRun manifest %q", manifestPath)

	var manifest map[string]any
	Expect(yaml.Unmarshal(contents, &manifest)).Should(Succeed(), "parsing PipelineRun manifest %q", manifestPath)

	spec := manifestObject(manifest["spec"], manifestPath, "spec")
	expectManifestParam(manifestList(spec["params"], manifestPath, "spec.params"), manifestPath, "build-platforms", []any{"linux/x86_64", "linux/arm64"})

	pipelineSpec := manifestObject(spec["pipelineSpec"], manifestPath, "spec.pipelineSpec")
	pipelineParams := manifestList(pipelineSpec["params"], manifestPath, "spec.pipelineSpec.params")
	buildPlatforms := namedManifestObject(pipelineParams, manifestPath, "build-platforms")
	Expect(buildPlatforms["type"]).Should(Equal("array"), "manifest %q: build-platforms must be an array parameter", manifestPath)

	tasks := manifestList(pipelineSpec["tasks"], manifestPath, "spec.pipelineSpec.tasks")
	taskNames := make([]string, 0, len(tasks))
	for _, task := range tasks {
		name, ok := manifestObject(task, manifestPath, "pipeline task")["name"].(string)
		Expect(ok).Should(BeTrue(), "manifest %q: pipeline task name should be a string", manifestPath)
		taskNames = append(taskNames, name)
	}
	Expect(taskNames).Should(ContainElement("build-images"), "manifest %q: pipeline must define build-images", manifestPath)
	Expect(taskNames).ShouldNot(ContainElement("build-container"), "manifest %q: single-architecture build-container task must be removed", manifestPath)

	buildImages := namedManifestObject(tasks, manifestPath, "build-images")
	matrix := manifestObject(buildImages["matrix"], manifestPath, "build-images.matrix")
	expectManifestParam(manifestList(matrix["params"], manifestPath, "build-images.matrix.params"), manifestPath, "PLATFORM", []any{"$(params.build-platforms)"})
	expectManifestParam(manifestList(buildImages["params"], manifestPath, "build-images.params"), manifestPath, "IMAGE_APPEND_PLATFORM", "true")
	expectTaskRefParam(buildImages, manifestPath, "name", "buildah-remote-oci-ta")
	expectTaskRefParam(buildImages, manifestPath, "bundle", remoteBuildahBundle)

	imageIndex := namedManifestObject(tasks, manifestPath, "build-image-index")
	expectManifestParam(manifestList(imageIndex["params"], manifestPath, "build-image-index.params"), manifestPath, "IMAGES", []any{"$(tasks.build-images.results.IMAGE_REF[*])"})
	Expect(manifestList(imageIndex["runAfter"], manifestPath, "build-image-index.runAfter")).Should(ContainElement("build-images"), "manifest %q: image index must run after build-images", manifestPath)
}

func manifestObject(value any, manifestPath, description string) map[string]any {
	object, ok := value.(map[string]any)
	Expect(ok).Should(BeTrue(), "manifest %q: %s should be a mapping", manifestPath, description)
	return object
}

func manifestList(value any, manifestPath, description string) []any {
	list, ok := value.([]any)
	Expect(ok).Should(BeTrue(), "manifest %q: %s should be a sequence", manifestPath, description)
	return list
}

func namedManifestObject(objects []any, manifestPath, name string) map[string]any {
	for _, value := range objects {
		object := manifestObject(value, manifestPath, "named object")
		if object["name"] == name {
			return object
		}
	}
	Fail(fmt.Sprintf("manifest %q: expected to find object named %q", manifestPath, name))
	return nil
}

func expectManifestParam(params []any, manifestPath, name string, expectedValue any) {
	param := namedManifestObject(params, manifestPath, name)
	Expect(param["value"]).Should(Equal(expectedValue), "manifest %q: parameter %q should have the expected value", manifestPath, name)
}

func expectTaskRefParam(task map[string]any, manifestPath, paramName, expectedValue string) {
	taskRef := manifestObject(task["taskRef"], manifestPath, "build-images.taskRef")
	params := manifestList(taskRef["params"], manifestPath, "build-images.taskRef.params")
	param := namedManifestObject(params, manifestPath, paramName)
	Expect(param["value"]).Should(Equal(expectedValue), "manifest %q: build-images taskRef parameter %q should match", manifestPath, paramName)
}
