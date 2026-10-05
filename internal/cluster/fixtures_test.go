package cluster

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func testFixtureSpec(provider, token string) FixtureSpec {
	return FixtureSpec{
		Namespace:   "tenant",
		RunID:       "run-1",
		Application: "app",
		Provider:    provider,
		GitLabToken: token,
		Components: []ComponentSpec{
			{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main"},
			{Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main"},
			{Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main"},
		},
	}
}

func fixtureTestClient(objects ...runtime.Object) *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		SecretGVR: "SecretList",
	}, objects...)
}

func TestFixtureServiceDoesNotCreateGitLabSecretForGitHub(t *testing.T) {
	client := fixtureTestClient()

	if _, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), testFixtureSpec("github", "token")); err != nil {
		t.Fatal(err)
	}

	secrets, err := client.Resource(SecretGVR).Namespace("tenant").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets.Items) != 0 {
		t.Fatalf("created %d Secrets for GitHub fixture; want none", len(secrets.Items))
	}
}

func TestFixtureServiceCreatesGitLabSCMSecret(t *testing.T) {
	client := fixtureTestClient()

	if _, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), testFixtureSpec("gitlab", "token")); err != nil {
		t.Fatal(err)
	}

	secret, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), gitLabCredentialSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := secret.GetLabels(); got[scmCredentialsLabel] != "scm" || got[scmHostnameLabel] != "gitlab.com" {
		t.Fatalf("Secret labels = %#v", got)
	}
	if got, found, _ := unstructured.NestedString(secret.Object, "type"); !found || got != string(corev1.SecretTypeBasicAuth) {
		t.Fatalf("Secret type = %q, found %t", got, found)
	}
	password, found, err := unstructured.NestedString(secret.Object, "stringData", "password")
	if err != nil {
		t.Fatal(err)
	}
	if !found || password != "token" {
		t.Fatalf("Secret password = %q, found %t", password, found)
	}
}

func TestFixtureServiceReusesExistingGitLabSCMSecret(t *testing.T) {
	client := fixtureTestClient(&unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      "existing-gitlab-secret",
			"namespace": "tenant",
			"labels": map[string]any{
				scmCredentialsLabel: "scm",
				scmHostnameLabel:    "gitlab.com",
			},
		},
		"type": "kubernetes.io/basic-auth",
		"data": map[string]any{"password": "existing-token"},
	}})

	if _, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), testFixtureSpec("gitlab", "new-token")); err != nil {
		t.Fatal(err)
	}

	if _, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), gitLabCredentialSecretName, metav1.GetOptions{}); err == nil {
		t.Fatal("created replacement GitLab Secret despite existing usable Secret")
	}
	secret, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), "existing-gitlab-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	password, found, err := unstructured.NestedString(secret.Object, "data", "password")
	if err != nil {
		t.Fatal(err)
	}
	if !found || password != "existing-token" {
		t.Fatalf("existing Secret password = %q, found %t", password, found)
	}
}

func TestFixtureServicePreservesUnrelatedExistingSecret(t *testing.T) {
	client := fixtureTestClient(&unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      "pipelines-as-code-secret",
			"namespace": "tenant",
		},
		"type": "Opaque",
		"data": map[string]any{"password": "unrelated-token"},
	}})

	if _, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), testFixtureSpec("gitlab", "token")); err != nil {
		t.Fatal(err)
	}

	if _, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), "pipelines-as-code-secret", metav1.GetOptions{}); err != nil {
		t.Fatalf("unrelated existing Secret was removed: %v", err)
	}
	if _, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), gitLabCredentialSecretName, metav1.GetOptions{}); err != nil {
		t.Fatalf("GitLab SCM Secret was not created: %v", err)
	}
}

func TestFixtureServiceSetsApplicationDisplayName(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	service := FixtureService{Dynamic: client}
	_, err := service.EnsureFixture(context.Background(), FixtureSpec{
		Namespace:   "tenant",
		RunID:       "run-1",
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
		RunID:       "run-1",
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
	spec := FixtureSpec{Namespace: "tenant", RunID: "run-1", Application: "app", Components: []ComponentSpec{{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main", Dockerfile: "one/Dockerfile"}, {Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main", Dockerfile: "two/Dockerfile"}, {Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main", Dockerfile: "three/Dockerfile"}}}
	_, err := service.EnsureFixture(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	spec.Components[0].Repository = "https://gitlab.com/other/repository"
	_, err = service.EnsureFixture(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "repository drift") {
		t.Fatalf("error = %v, want repository drift", err)
	}
}

func TestFixtureServiceLabelsCreatedNamespaceWithOwnership(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	_, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), FixtureSpec{
		Namespace: "tenant", RunID: "run-1", Application: "app",
		Components: []ComponentSpec{{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := client.Resource(NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if namespace.GetLabels()["app.konflux-ci.org/managed-by"] != "konflux-test" || namespace.GetLabels()["app.konflux.org/run-id"] != "run-1" || namespace.GetLabels()["konflux-ci.dev/type"] != "tenant" || namespace.GetLabels()["cost-center"] != "670" {
		t.Fatalf("namespace labels = %#v", namespace.GetLabels())
	}
}
