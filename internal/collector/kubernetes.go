package collector

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type resourceSource struct {
	name      string
	gvr       schema.GroupVersionResource
	namespace string
	selector  string
	path      string
}

func KubernetesSources(client dynamic.Interface, namespace, _ string) []Source {
	resources := []resourceSource{
		{name: "applications", gvr: schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}, namespace: namespace, path: "workload/applications.json"},
		{name: "components", gvr: schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"}, namespace: namespace, path: "workload/components.json"},
		{name: "pipelineruns", gvr: schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}, namespace: namespace, path: "workload/pipelineruns.json"},
		{name: "taskruns", gvr: schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}, namespace: namespace, path: "workload/taskruns.json"},
		{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, namespace: namespace, path: "workload/pods.json"},
	}
	sources := make([]Source, 0, len(resources))
	for _, resource := range resources {
		resource := resource
		sources = append(sources, Source{Name: resource.name, Path: resource.path, Collect: func(ctx context.Context, root string) error {
			if client == nil {
				return fmt.Errorf("dynamic client is required")
			}
			objects, err := client.Resource(resource.gvr).Namespace(resource.namespace).List(ctx, metav1.ListOptions{LabelSelector: resource.selector})
			if err != nil {
				return err
			}
			return writeJSON(filepath.Join(root, resource.path), objects)
		}})
	}
	return sources
}

var DefaultLogNamespaces = []string{
	"pipelines-as-code",
	"tekton-pipelines",
	"multi-platform-controller",
	"konflux-image-controller",
}

func KubernetesPodLogSources(client kubernetes.Interface, tenantNamespace string, startedAt time.Time) []Source {
	namespaces := append([]string{tenantNamespace}, DefaultLogNamespaces...)
	seen := make(map[string]struct{}, len(namespaces))
	sources := make([]Source, 0, len(namespaces))
	for _, namespace := range namespaces {
		if strings.TrimSpace(namespace) == "" {
			continue
		}
		if _, ok := seen[namespace]; ok {
			continue
		}
		seen[namespace] = struct{}{}
		sources = append(sources, newPodLogSource(client, namespace, startedAt, nil))
	}
	return sources
}

type podLogFetcher func(context.Context, string, string, string, corev1.PodLogOptions) ([]byte, error)

func newPodLogSource(client kubernetes.Interface, namespace string, startedAt time.Time, fetch podLogFetcher) Source {
	path := filepath.Join("logs", namespace)
	if fetch == nil {
		fetch = func(ctx context.Context, namespace, pod, container string, options corev1.PodLogOptions) ([]byte, error) {
			if client == nil {
				return nil, fmt.Errorf("kubernetes client is required")
			}
			stream, err := client.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{Container: container, SinceTime: options.SinceTime, Timestamps: options.Timestamps}).Stream(ctx)
			if err != nil {
				return nil, err
			}
			defer stream.Close()
			return io.ReadAll(stream)
		}
	}
	return Source{Name: "pod-logs/" + namespace, Path: path, Optional: true, Collect: func(ctx context.Context, root string) error {
		if client == nil {
			return fmt.Errorf("kubernetes client is required")
		}
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		options := corev1.PodLogOptions{SinceTime: &metav1.Time{Time: startedAt}, Timestamps: true}
		for _, pod := range pods.Items {
			containers := append([]corev1.Container(nil), pod.Spec.InitContainers...)
			containers = append(containers, pod.Spec.Containers...)
			for _, container := range containers {
				data, err := fetch(ctx, namespace, pod.Name, container.Name, options)
				if err != nil {
					return fmt.Errorf("get logs for %s/%s: %w", pod.Name, container.Name, err)
				}
				logPath := filepath.Join(root, path, safePathPart(pod.Name), safePathPart(container.Name)+".log")
				if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
					return err
				}
				if err := os.WriteFile(logPath, data, 0o640); err != nil {
					return err
				}
			}
		}
		return nil
	}}
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
