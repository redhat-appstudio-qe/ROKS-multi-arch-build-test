package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	PipelineRunGVR = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}
	TaskRunGVR     = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}
)

type BuildOutputInspector struct {
	Dynamic    dynamic.Interface
	Kubernetes kubernetes.Interface
	Now        func() time.Time
}

func (i BuildOutputInspector) Verify(ctx context.Context, identities []model.PipelineRunIdentity) ([]model.BuildOutputEvidence, error) {
	if i.Dynamic == nil {
		return nil, fmt.Errorf("dynamic client is required")
	}
	if i.Kubernetes == nil {
		return nil, fmt.Errorf("kubernetes client is required for Pod and Node build evidence")
	}
	if len(identities) == 0 {
		return nil, fmt.Errorf("at least one PipelineRun is required")
	}
	now := time.Now
	if i.Now != nil {
		now = i.Now
	}
	evidence := make([]model.BuildOutputEvidence, 0, len(identities))
	for _, identity := range identities {
		output, err := i.verifyPipelineRun(ctx, identity, now().UTC())
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, output)
	}
	return evidence, nil
}

func (i BuildOutputInspector) verifyPipelineRun(ctx context.Context, identity model.PipelineRunIdentity, verifiedAt time.Time) (model.BuildOutputEvidence, error) {
	if strings.TrimSpace(identity.Namespace) == "" || strings.TrimSpace(identity.Name) == "" {
		return model.BuildOutputEvidence{}, fmt.Errorf("PipelineRun identity is incomplete")
	}
	pipelineRuns := i.Dynamic.Resource(PipelineRunGVR).Namespace(identity.Namespace)
	pipelineRun, err := pipelineRuns.Get(ctx, identity.Name, metav1.GetOptions{})
	if err != nil {
		return model.BuildOutputEvidence{}, fmt.Errorf("get PipelineRun %s/%s: %w", identity.Namespace, identity.Name, err)
	}
	if identity.UID != "" && pipelineRun.GetUID() != identity.UID {
		return model.BuildOutputEvidence{}, fmt.Errorf("PipelineRun %s/%s identity changed", identity.Namespace, identity.Name)
	}
	if conditionStatus(pipelineRun) != "True" {
		return model.BuildOutputEvidence{}, fmt.Errorf("PipelineRun %s/%s did not succeed", identity.Namespace, identity.Name)
	}
	if identity.Component == "" {
		return model.BuildOutputEvidence{}, fmt.Errorf("PipelineRun %s/%s has no component identity", identity.Namespace, identity.Name)
	}
	taskRuns, err := i.Dynamic.Resource(TaskRunGVR).Namespace(identity.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.BuildOutputEvidence{}, fmt.Errorf("list TaskRuns for PipelineRun %s/%s: %w", identity.Namespace, identity.Name, err)
	}
	createdOutputs := make(map[string]string)
	outputDigests := make(map[string]string)
	for index := range taskRuns.Items {
		taskRun := &taskRuns.Items[index]
		if !ownedByPipelineRun(taskRun, string(pipelineRun.GetUID()), pipelineRun.GetName()) {
			continue
		}
		if conditionStatus(taskRun) == "False" {
			return model.BuildOutputEvidence{}, fmt.Errorf("build TaskRun %s/%s failed", taskRun.GetNamespace(), taskRun.GetName())
		}
		if conditionStatus(taskRun) != "True" {
			continue
		}
		if !isBuildTaskRun(taskRun) {
			continue
		}
		platform, output, digest := taskRunOutput(taskRun)
		if output == "" || digest == "" {
			return model.BuildOutputEvidence{}, fmt.Errorf("build TaskRun %s/%s is missing IMAGE_URL or IMAGE_DIGEST", taskRun.GetNamespace(), taskRun.GetName())
		}
		if i.Kubernetes != nil || platform == "" {
			platform, err = i.taskRunPodPlatform(ctx, taskRun)
			if err != nil {
				return model.BuildOutputEvidence{}, err
			}
		}
		if platform == "" {
			return model.BuildOutputEvidence{}, fmt.Errorf("build TaskRun %s/%s output has no platform evidence", taskRun.GetNamespace(), taskRun.GetName())
		}
		createdOutputs[platform] = output
		outputDigests[platform] = digest
	}
	platforms := make([]string, 0, len(createdOutputs))
	for _, platform := range []string{"linux/amd64", "linux/arm64"} {
		if createdOutputs[platform] != "" {
			platforms = append(platforms, platform)
		}
	}
	if len(platforms) != 2 {
		return model.BuildOutputEvidence{}, fmt.Errorf("PipelineRun %s/%s build outputs do not prove linux/amd64 and linux/arm64", identity.Namespace, identity.Name)
	}
	verifiedIdentity := identity
	verifiedIdentity.UID = pipelineRun.GetUID()
	verifiedIdentity.Succeeded = true
	return model.BuildOutputEvidence{Component: identity.Component, PipelineRun: verifiedIdentity, Platforms: platforms, CreatedOutputs: createdOutputs, OutputDigests: outputDigests, VerifiedAt: verifiedAt}, nil
}

func ownedByPipelineRun(taskRun *unstructured.Unstructured, uid, name string) bool {
	owners, found, err := unstructured.NestedSlice(taskRun.Object, "metadata", "ownerReferences")
	if err != nil || !found {
		return false
	}
	for _, raw := range owners {
		owner, ok := raw.(map[string]any)
		if !ok || owner["kind"] != "PipelineRun" {
			continue
		}
		ownerUID, _ := owner["uid"].(string)
		ownerName, _ := owner["name"].(string)
		if (uid != "" && ownerUID == uid) || (uid == "" && ownerName == name) {
			return true
		}
	}
	return false
}

func conditionStatus(object *unstructured.Unstructured) string {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok || condition["type"] != "Succeeded" {
			continue
		}
		status, _ := condition["status"].(string)
		return status
	}
	return "Unknown"
}

func isBuildTaskRun(taskRun *unstructured.Unstructured) bool {
	labels := taskRun.GetLabels()
	if labels["tekton.dev/pipelineTask"] != "build-container" {
		return false
	}
	if strings.Contains(strings.ToLower(labels["tekton.dev/task"]), "buildah") {
		return true
	}
	params, _, _ := unstructured.NestedSlice(taskRun.Object, "spec", "taskRef", "params")
	for _, raw := range params {
		param, ok := raw.(map[string]any)
		if !ok || param["name"] != "name" {
			continue
		}
		value, _ := param["value"].(string)
		return strings.Contains(strings.ToLower(value), "buildah")
	}
	return false
}

func taskRunOutput(taskRun *unstructured.Unstructured) (string, string, string) {
	platform := taskRunPlatform(taskRun)
	imageURL := ""
	digest := ""
	results, _, _ := unstructured.NestedSlice(taskRun.Object, "status", "results")
	for _, raw := range results {
		result, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := result["name"].(string)
		value, _ := result["value"].(string)
		if strings.Contains(strings.ToUpper(name), "PLATFORM") && isBuildPlatform(value) {
			platform = value
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		switch strings.ToUpper(name) {
		case "IMAGE_URL":
			imageURL = value
		case "IMAGE_DIGEST":
			digest = value
		}
	}
	if !isBuildPlatform(platform) {
		platform = ""
	}
	return platform, imageURL, digest
}

func taskRunPlatform(taskRun *unstructured.Unstructured) string {
	for _, values := range []map[string]string{taskRun.GetLabels(), taskRun.GetAnnotations()} {
		for _, key := range []string{
			"build.appstudio.redhat.com/platform",
			"build.appstudio.openshift.io/platform",
		} {
			if platform := strings.TrimSpace(values[key]); isBuildPlatform(platform) {
				return platform
			}
		}
	}
	params, _, _ := unstructured.NestedSlice(taskRun.Object, "spec", "params")
	for _, raw := range params {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := param["name"].(string)
		if strings.EqualFold(name, "PLATFORM") {
			if platform, ok := param["value"].(string); ok && isBuildPlatform(platform) {
				return platform
			}
		}
	}
	return ""
}

func (i BuildOutputInspector) taskRunPodPlatform(ctx context.Context, taskRun *unstructured.Unstructured) (string, error) {
	if i.Kubernetes == nil {
		if platform := taskRunPlatform(taskRun); platform != "" {
			return platform, nil
		}
		return taskRunPodTemplatePlatform(taskRun), nil
	}
	podName, _, _ := unstructured.NestedString(taskRun.Object, "status", "podName")
	if strings.TrimSpace(podName) == "" {
		return "", nil
	}
	pod, err := i.Kubernetes.CoreV1().Pods(taskRun.GetNamespace()).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get build TaskRun pod %s/%s: %w", taskRun.GetNamespace(), podName, err)
	}
	if strings.TrimSpace(pod.Spec.NodeName) == "" {
		return "", nil
	}
	node, err := i.Kubernetes.CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get build TaskRun node %s: %w", pod.Spec.NodeName, err)
	}
	architecture := node.Labels["kubernetes.io/arch"]
	if architecture == "" {
		architecture = node.Status.NodeInfo.Architecture
	}
	return platformFromArchitecture(architecture), nil
}

func taskRunPodTemplatePlatform(taskRun *unstructured.Unstructured) string {
	nodeSelector, _, _ := unstructured.NestedStringMap(taskRun.Object, "spec", "podTemplate", "nodeSelector")
	return platformFromArchitecture(nodeSelector["kubernetes.io/arch"])
}

func platformFromArchitecture(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "linux/amd64", "linux/arm64":
		return value
	case "amd64":
		return "linux/amd64"
	case "arm64":
		return "linux/arm64"
	default:
		return ""
	}
}

func isBuildPlatform(value string) bool {
	return value == "linux/amd64" || value == "linux/arm64"
}
