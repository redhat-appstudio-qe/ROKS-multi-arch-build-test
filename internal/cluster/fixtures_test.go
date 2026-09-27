package cluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestFixtureServiceSetsApplicationDisplayName(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	service := FixtureService{Dynamic: client}
	_, err := service.EnsureFixture(context.Background(), FixtureSpec{
		Namespace:   "tenant",
		Application: "app",
		Components: []ComponentSpec{
			{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main"},
			{Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main"},
			{Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	application, err := client.Resource(ApplicationGVR).Namespace("tenant").Get(context.Background(), "app", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	displayName, found, err := unstructured.NestedString(application.Object, "spec", "displayName")
	if err != nil {
		t.Fatal(err)
	}
	if !found || displayName != "app" {
		t.Fatalf("application display name = %q, found %t; want %q", displayName, found, "app")
	}
}

func TestFixtureServiceConfiguresBuildAndImageControllers(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	service := FixtureService{Dynamic: client}
	_, err := service.EnsureFixture(context.Background(), FixtureSpec{
		Namespace:   "tenant",
		Application: "app",
		Components: []ComponentSpec{
			{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main"},
			{Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main"},
			{Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"one", "two", "three"} {
		component, err := client.Resource(ComponentGVR).Namespace("tenant").Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		annotations := component.GetAnnotations()
		if annotations["build.appstudio.openshift.io/request"] != "configure-pac" {
			t.Errorf("component %s PaC request = %q; want %q", component.GetName(), annotations["build.appstudio.openshift.io/request"], "configure-pac")
		}
		if annotations["image.redhat.com/generate"] != `{"visibility":"public"}` {
			t.Errorf("component %s image generation request = %q; want public repository request", component.GetName(), annotations["image.redhat.com/generate"])
		}
	}
}

func TestFixtureServiceRejectsDrift(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	service := FixtureService{Dynamic: client}
	_, err := service.EnsureFixture(context.Background(), FixtureSpec{Namespace: "tenant", Application: "app", Components: []ComponentSpec{{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.EnsureFixture(context.Background(), FixtureSpec{Namespace: "tenant", Application: "app", Components: []ComponentSpec{{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
}
