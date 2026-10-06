package cluster

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

var _ = Describe("FixtureService", func() {
	It("splits component path into build context and Dockerfile", func() {
		client := fixtureTestClient()
		spec := testFixtureSpec("github", "token")
		spec.Components[0].Context = "web-server"
		spec.Components[0].Dockerfile = "Dockerfile"
		spec.Components[1].Context = "history-worker"
		spec.Components[1].Dockerfile = "Dockerfile"
		spec.Components[2].Context = "frontend"
		spec.Components[2].Dockerfile = "Dockerfile"

		_, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), spec)
		Expect(err).NotTo(HaveOccurred())
		for _, expected := range []struct {
			name    string
			context string
		}{
			{name: "one", context: "web-server"},
			{name: "two", context: "history-worker"},
			{name: "three", context: "frontend"},
		} {
			component, err := client.Resource(ComponentGVR).Namespace("tenant").Get(context.Background(), expected.name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			gotContext, found, err := unstructured.NestedString(component.Object, "spec", "source", "git", "context")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(gotContext).To(Equal(expected.context))
			gotDockerfile, found, err := unstructured.NestedString(component.Object, "spec", "source", "git", "dockerfileUrl")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(gotDockerfile).To(Equal("Dockerfile"))
		}
	})

	It("does not create GitLab Secret for GitHub", func() {
		client := fixtureTestClient()

		if _, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), testFixtureSpec("github", "token")); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		secrets, err := client.Resource(SecretGVR).Namespace("tenant").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(secrets.Items).To(BeEmpty())
	})

	It("creates GitLab SCM Secret", func() {
		client := fixtureTestClient()

		if _, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), testFixtureSpec("gitlab", "token")); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		secret, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), gitLabCredentialSecretName, metav1.GetOptions{})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		got := secret.GetLabels()
		Expect(got).To(SatisfyAll(HaveKeyWithValue(scmCredentialsLabel, "scm"), HaveKeyWithValue(scmHostnameLabel, "gitlab.com")))
		if got, found, _ := unstructured.NestedString(secret.Object, "type"); !found || got != string(corev1.SecretTypeBasicAuth) {
			Expect(found).To(BeTrue())
			Expect(got).To(Equal(string(corev1.SecretTypeBasicAuth)))
		}
		password, found, err := unstructured.NestedString(secret.Object, "stringData", "password")
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(found).To(BeTrue())
		Expect(password).To(Equal("token"))
	})

	It("reuses existing GitLab SCM Secret", func() {
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
			Expect(err).NotTo(HaveOccurred())
		}

		if _, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), gitLabCredentialSecretName, metav1.GetOptions{}); err == nil {
			Expect(err).To(HaveOccurred())
		}
		secret, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), "existing-gitlab-secret", metav1.GetOptions{})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		password, found, err := unstructured.NestedString(secret.Object, "data", "password")
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(found).To(BeTrue())
		Expect(password).To(Equal("existing-token"))
	})

	It("preserves unrelated existing Secret", func() {
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
			Expect(err).NotTo(HaveOccurred())
		}

		if _, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), "pipelines-as-code-secret", metav1.GetOptions{}); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		_, err := client.Resource(SecretGVR).Namespace("tenant").Get(context.Background(), gitLabCredentialSecretName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("sets application display name", func() {
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
			Expect(err).NotTo(HaveOccurred())
		}

		application, err := client.Resource(ApplicationGVR).Namespace("tenant").Get(context.Background(), "app", metav1.GetOptions{})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		displayName, found, err := unstructured.NestedString(application.Object, "spec", "displayName")
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(found).To(BeTrue())
		Expect(displayName).To(Equal("app"))
	})

	It("configures build and image controllers", func() {
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
			Expect(err).NotTo(HaveOccurred())
		}

		for _, name := range []string{"one", "two", "three"} {
			component, err := client.Resource(ComponentGVR).Namespace("tenant").Get(context.Background(), name, metav1.GetOptions{})
			if err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
			annotations := component.GetAnnotations()
			Expect(annotations).To(HaveKeyWithValue("build.appstudio.openshift.io/request", "configure-pac"))
			Expect(annotations).To(HaveKeyWithValue("image.redhat.com/generate", `{"visibility":"public"}`))
		}
	})

	It("rejects drift", func() {
		client := fake.NewSimpleDynamicClient(runtime.NewScheme())
		service := FixtureService{Dynamic: client}
		spec := FixtureSpec{Namespace: "tenant", RunID: "run-1", Application: "app", Components: []ComponentSpec{{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main", Dockerfile: "one/Dockerfile"}, {Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main", Dockerfile: "two/Dockerfile"}, {Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main", Dockerfile: "three/Dockerfile"}}}
		_, err := service.EnsureFixture(context.Background(), spec)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		spec.Components[0].Repository = "https://gitlab.com/other/repository"
		_, err = service.EnsureFixture(context.Background(), spec)
		Expect(err).To(MatchError(ContainSubstring("repository drift")))
	})

	It("labels created namespace with ownership", func() {
		client := fake.NewSimpleDynamicClient(runtime.NewScheme())
		_, err := (FixtureService{Dynamic: client}).EnsureFixture(context.Background(), FixtureSpec{
			Namespace: "tenant", RunID: "run-1", Application: "app",
			Components: []ComponentSpec{{Name: "one", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "two", Repository: "https://gitlab.com/o/r", Branch: "main"}, {Name: "three", Repository: "https://gitlab.com/o/r", Branch: "main"}},
		})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		namespace, err := client.Resource(NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(namespace.GetLabels()).To(SatisfyAll(HaveKeyWithValue("app.konflux-ci.org/managed-by", "konflux-test"), HaveKeyWithValue("app.konflux.org/run-id", "run-1"), HaveKeyWithValue("konflux-ci.dev/type", "tenant"), HaveKeyWithValue("cost-center", "670")))
	})
})
