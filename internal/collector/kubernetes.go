package collector

import (
	"context"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
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
