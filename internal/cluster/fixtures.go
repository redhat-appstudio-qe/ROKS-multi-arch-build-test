package cluster

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	NamespaceGVR   = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	ApplicationGVR = schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "applications"}
	ComponentGVR   = schema.GroupVersionResource{Group: "appstudio.redhat.com", Version: "v1alpha1", Resource: "components"}
)

const (
	componentPaCRequestAnnotation = "build.appstudio.openshift.io/request"
	imageGenerationAnnotation     = "image.redhat.com/generate"
)

type ComponentSpec struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Context    string `json:"context"`
	Dockerfile string `json:"dockerfile"`
}

type FixtureSpec struct {
	Namespace   string
	Application string
	Components  []ComponentSpec
}

type FixtureResult struct {
	Namespace   string
	Application string
	Components  []string
}

type FixtureService struct {
	Dynamic dynamic.Interface
}

func (s FixtureService) EnsureFixture(ctx context.Context, spec FixtureSpec) (FixtureResult, error) {
	if s.Dynamic == nil {
		return FixtureResult{}, fmt.Errorf("dynamic client is required")
	}
	if spec.Namespace == "" || spec.Application == "" || len(spec.Components) != 3 {
		return FixtureResult{}, fmt.Errorf("fixture requires namespace, application, and exactly three components")
	}
	namespaces := s.Dynamic.Resource(NamespaceGVR)
	if _, err := namespaces.Get(ctx, spec.Namespace, metav1.GetOptions{}); errors.IsNotFound(err) {
		object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": spec.Namespace, "labels": map[string]any{"app.konflux-ci.org/managed-by": "konflux-test"}}}}
		if _, err := namespaces.Create(ctx, object, metav1.CreateOptions{}); err != nil {
			return FixtureResult{}, fmt.Errorf("create namespace: %w", err)
		}
	} else if err != nil {
		return FixtureResult{}, fmt.Errorf("get namespace: %w", err)
	}
	applications := s.Dynamic.Resource(ApplicationGVR).Namespace(spec.Namespace)
	application, err := applications.Get(ctx, spec.Application, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		application = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "appstudio.redhat.com/v1alpha1", "kind": "Application", "metadata": map[string]any{"name": spec.Application, "namespace": spec.Namespace}, "spec": map[string]any{"displayName": spec.Application}}}
		if application, err = applications.Create(ctx, application, metav1.CreateOptions{}); err != nil {
			return FixtureResult{}, fmt.Errorf("create application: %w", err)
		}
	} else if err != nil {
		return FixtureResult{}, fmt.Errorf("get application: %w", err)
	}
	if application.GetNamespace() != spec.Namespace {
		return FixtureResult{}, fmt.Errorf("application drift: namespace %q", application.GetNamespace())
	}
	components := s.Dynamic.Resource(ComponentGVR).Namespace(spec.Namespace)
	result := FixtureResult{Namespace: spec.Namespace, Application: spec.Application}
	for _, component := range spec.Components {
		if strings.TrimSpace(component.Name) == "" || strings.TrimSpace(component.Repository) == "" || strings.TrimSpace(component.Branch) == "" {
			return FixtureResult{}, fmt.Errorf("component %q is incomplete", component.Name)
		}
		object, getErr := components.Get(ctx, component.Name, metav1.GetOptions{})
		if errors.IsNotFound(getErr) {
			object = &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "appstudio.redhat.com/v1alpha1", "kind": "Component",
				"metadata": map[string]any{
					"name": component.Name, "namespace": spec.Namespace,
					"annotations": map[string]any{
						componentPaCRequestAnnotation: "configure-pac",
						imageGenerationAnnotation:     `{"visibility":"public"}`,
					},
				},
				"spec": map[string]any{"application": spec.Application, "source": map[string]any{"git": map[string]any{"url": component.Repository, "revision": component.Branch, "context": component.Context, "dockerfileUrl": component.Dockerfile}}},
			}}
			if _, getErr = components.Create(ctx, object, metav1.CreateOptions{}); getErr != nil {
				return FixtureResult{}, fmt.Errorf("create component %s: %w", component.Name, getErr)
			}
		} else if getErr != nil {
			return FixtureResult{}, fmt.Errorf("get component %s: %w", component.Name, getErr)
		} else if err := validateComponent(object, component, spec.Application); err != nil {
			return FixtureResult{}, err
		}
		result.Components = append(result.Components, component.Name)
	}
	return result, nil
}

func validateComponent(object *unstructured.Unstructured, expected ComponentSpec, application string) error {
	actualApplication, _, _ := unstructured.NestedString(object.Object, "spec", "application")
	if actualApplication != "" && actualApplication != application {
		return fmt.Errorf("component %s drift: application %q", expected.Name, actualApplication)
	}
	return nil
}
