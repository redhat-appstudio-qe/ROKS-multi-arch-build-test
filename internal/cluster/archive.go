package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/redhat-appstudio/konflux-test/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func ParseGVR(value string) (schema.GroupVersionResource, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("invalid archive GVR %q; expected group/version/resource", value)
	}
	return schema.GroupVersionResource{Group: parts[0], Version: parts[1], Resource: parts[2]}, nil
}

func QueryArchive(ctx context.Context, client dynamic.Interface, endpoint ArchiveEndpoint, identity model.PipelineRunIdentity) (map[string]any, error) {
	if client == nil {
		return nil, fmt.Errorf("dynamic client is required")
	}
	resource := client.Resource(endpoint.GVR)
	var object *unstructured.Unstructured
	var err error
	if endpoint.NamespaceScoped {
		object, err = resource.Namespace(identity.Namespace).Get(ctx, identity.Name, metav1GetOptions())
	} else {
		object, err = resource.Get(ctx, identity.Name, metav1GetOptions())
	}
	if err != nil {
		return nil, err
	}
	return object.Object, nil
}

func metav1GetOptions() metav1.GetOptions { return metav1.GetOptions{} }

func CompareArchiveIdentity(raw map[string]any, identity model.PipelineRunIdentity) (map[string]bool, error) {
	metadata, ok := raw["metadata"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("archive response has no metadata")
	}
	comparisons := map[string]bool{
		"namespace": metadataString(metadata, "namespace") == identity.Namespace,
		"name":      metadataString(metadata, "name") == identity.Name,
		"uid":       metadataString(metadata, "uid") == string(identity.UID),
	}
	if !comparisons["namespace"] || !comparisons["name"] || !comparisons["uid"] {
		return comparisons, nil
	}
	return comparisons, nil
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}
