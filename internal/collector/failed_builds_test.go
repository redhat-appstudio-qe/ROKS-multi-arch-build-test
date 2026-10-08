package collector_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/collector"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("FailedBuildLog", func() {
	It("has all required fields", func() {
		log := collector.FailedBuildLog{
			PipelineRunName:  "pr-1",
			TaskRunName:      "tr-1",
			TaskName:         "build-container",
			StepContainer:    "step-build",
			ExitCode:         1,
			ConditionMessage: "failed",
		}
		Expect(log.PipelineRunName).To(Equal("pr-1"))
		Expect(log.TaskRunName).To(Equal("tr-1"))
		Expect(log.TaskName).To(Equal("build-container"))
		Expect(log.StepContainer).To(Equal("step-build"))
		Expect(log.ExitCode).To(Equal(int64(1)))
		Expect(log.ConditionMessage).To(Equal("failed"))
	})
})

var _ = Describe("FindFailedStepContainers", func() {
	It("identifies steps with non-zero exit codes", func() {
		taskRun := taskRunWithSteps(map[string]any{
			"name": "build", "container": "step-build",
			"terminated": map[string]any{"exitCode": int64(1), "reason": "Error"},
		}, map[string]any{
			"name": "push", "container": "step-push",
			"terminated": map[string]any{"exitCode": int64(0), "reason": "Completed"},
		})

		failed := collector.FindFailedStepContainers(taskRun)
		Expect(failed).To(HaveLen(1))
		Expect(failed[0].Container).To(Equal("step-build"))
		Expect(failed[0].ExitCode).To(Equal(int64(1)))
	})

	It("returns empty when all steps succeed", func() {
		taskRun := taskRunWithSteps(map[string]any{
			"name": "build", "container": "step-build",
			"terminated": map[string]any{"exitCode": int64(0), "reason": "Completed"},
		})

		Expect(collector.FindFailedStepContainers(taskRun)).To(BeEmpty())
	})

	It("falls back to step name when container is empty", func() {
		taskRun := taskRunWithSteps(map[string]any{
			"name": "build", "container": "",
			"terminated": map[string]any{"exitCode": int64(1), "reason": "Error"},
		})

		failed := collector.FindFailedStepContainers(taskRun)
		Expect(failed).To(HaveLen(1))
		Expect(failed[0].Container).To(Equal("step-build"))
	})
})

var _ = Describe("CollectFailedBuildLogs", func() {
	It("writes container logs to files", func() {
		taskRun := failedTaskRun()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			runtime.NewScheme(),
			map[schema.GroupVersionResource]string{collector.TaskRunGVR: "TaskRunList"},
			taskRun,
		)
		kubeClient := kubernetesfake.NewSimpleClientset(&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "build-pod-amd64", Namespace: "tenant"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "step-build"}}},
		})

		outputDir := GinkgoT().TempDir()
		logs, err := (collector.FailedBuildCollector{
			Dynamic:    dynamicClient,
			Kubernetes: kubeClient,
			TailLines:  80,
		}).CollectFailedBuildLogs(context.Background(), "tenant", "build-pr-1", outputDir)

		Expect(err).NotTo(HaveOccurred())
		Expect(logs).To(HaveLen(1))
		Expect(logs[0].PipelineRunName).To(Equal("build-pr-1"))
		Expect(logs[0].TaskRunName).To(Equal("build-task-amd64"))
		Expect(logs[0].TaskName).To(Equal("build-container"))
		Expect(logs[0].StepContainer).To(Equal("step-build"))
		Expect(logs[0].ExitCode).To(Equal(int64(1)))
		Expect(logs[0].LogPath).To(Equal(filepath.Join("failed-builds", "build-task-amd64_step-build.log")))
		_, err = os.ReadFile(filepath.Join(outputDir, logs[0].LogPath))
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns empty when no TaskRuns failed", func() {
		taskRun := failedTaskRun()
		status := taskRun.Object["status"].(map[string]any)
		status["conditions"] = []any{map[string]any{"type": "Succeeded", "status": "True"}}
		status["steps"] = []any{map[string]any{
			"name": "build", "container": "step-build",
			"terminated": map[string]any{"exitCode": int64(0), "reason": "Completed"},
		}}
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			runtime.NewScheme(),
			map[schema.GroupVersionResource]string{collector.TaskRunGVR: "TaskRunList"},
			taskRun,
		)

		logs, err := (collector.FailedBuildCollector{Dynamic: dynamicClient}).CollectFailedBuildLogs(
			context.Background(), "tenant", "build-pr-1", GinkgoT().TempDir(),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).To(BeEmpty())
	})
})

func taskRunWithSteps(steps ...map[string]any) *unstructured.Unstructured {
	objects := make([]any, len(steps))
	for index, step := range steps {
		objects[index] = step
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "TaskRun",
		"metadata": map[string]any{
			"name": "tr-1", "namespace": "tenant",
		},
		"status": map[string]any{"steps": objects},
	}}
}

func failedTaskRun() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "TaskRun",
		"metadata": map[string]any{
			"name": "build-task-amd64", "namespace": "tenant",
			"labels": map[string]any{
				"tekton.dev/pipelineRun":  "build-pr-1",
				"tekton.dev/pipelineTask": "build-container",
			},
		},
		"status": map[string]any{
			"podName": "build-pod-amd64",
			"conditions": []any{map[string]any{
				"type": "Succeeded", "status": "False", "message": "step failed",
			}},
			"steps": []any{map[string]any{
				"name": "build", "container": "step-build",
				"terminated": map[string]any{"exitCode": int64(1), "reason": "Error"},
			}},
		},
	}}
}
