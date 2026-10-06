package cleanup

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

func StripFinalizers(ctx context.Context, client dynamic.Interface, namespace string, gvr schema.GroupVersionResource) (int, error) {
	if client == nil {
		return 0, fmt.Errorf("dynamic client is required")
	}

	list, err := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("list %s for finalizer stripping: %w", gvr.Resource, err)
	}

	stripped := 0
	for index := range list.Items {
		item := &list.Items[index]
		if len(item.GetFinalizers()) == 0 {
			continue
		}
		if _, err := client.Resource(gvr).Namespace(namespace).Patch(ctx, item.GetName(), types.MergePatchType, []byte(`{"metadata":{"finalizers":null}}`), metav1.PatchOptions{}); err != nil {
			return stripped, fmt.Errorf("strip finalizers from %s/%s: %w", gvr.Resource, item.GetName(), err)
		}
		stripped++
	}
	return stripped, nil
}

var finalizerGVRs = []schema.GroupVersionResource{
	{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"},
	{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"},
	{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "imagerepositories"},
	{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "releases"},
	{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"},
}

func StripAllFinalizers(ctx context.Context, client dynamic.Interface, namespace string) (int, error) {
	total := 0
	var firstErr error
	for _, gvr := range finalizerGVRs {
		count, err := StripFinalizers(ctx, client, namespace, gvr)
		total += count
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return total, firstErr
}
