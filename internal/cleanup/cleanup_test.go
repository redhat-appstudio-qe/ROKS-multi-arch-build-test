package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestNamespaceServiceDeletesOnlyExactOwnedNamespace(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespace("tenant", "run-1", true))
	service := NamespaceService{Dynamic: client}
	if err := service.Delete(context.Background(), "tenant", "run-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.WaitDeleted(context.Background(), "tenant"); err != nil {
		t.Fatal(err)
	}
}

func TestNamespaceServiceRefusesUnownedNamespace(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespace("tenant", "run-1", false))
	err := (NamespaceService{Dynamic: client}).Delete(context.Background(), "tenant", "run-1")
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("error = %v", err)
	}
	if _, err := client.Resource(NamespaceGVR).Get(context.Background(), "tenant", metav1.GetOptions{}); err != nil {
		t.Fatal("namespace was deleted")
	}
}

func TestNamespaceServiceRequiresRunIDForDeletion(t *testing.T) {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), namespace("tenant", "run-1", true))
	if err := (NamespaceService{Dynamic: client}).Delete(context.Background(), "tenant", ""); err == nil {
		t.Fatal("expected missing run ID rejection")
	}
}

func TestNamespaceServiceFindCandidatesUsesCLINamesAndOwnershipSelector(t *testing.T) {
	client := fake.NewSimpleDynamicClient(
		runtime.NewScheme(),
		namespace("mathwizz-test-github", "run-1", true),
		namespace("mathwizz-test-github-20261005b", "run-2", true),
		namespace("other-tenant", "run-3", true),
	)
	service := NamespaceService{
		Dynamic:    client,
		NameLister: staticNameLister{names: []string{"mathwizz-test-github", "mathwizz-test-github-20261005b"}},
	}

	candidates, err := service.FindCandidates(context.Background(), "mathwizz-test-github")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].Name != "mathwizz-test-github" || candidates[1].Name != "mathwizz-test-github-20261005b" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestNamespaceServiceFindCandidatesRejectsUnownedCLIName(t *testing.T) {
	client := fake.NewSimpleDynamicClient(
		runtime.NewScheme(),
		namespace("mathwizz-test-github", "run-1", true),
		namespace("mathwizz-test-github-20261005b", "run-2", false),
	)
	service := NamespaceService{
		Dynamic:    client,
		NameLister: staticNameLister{names: []string{"mathwizz-test-github", "mathwizz-test-github-20261005b"}},
	}

	_, err := service.FindCandidates(context.Background(), "mathwizz-test-github")
	if err == nil || !strings.Contains(err.Error(), "without exact konflux-test ownership labels") {
		t.Fatalf("error = %v", err)
	}
}

func TestOCNamespaceNameListerUsesCLIOutput(t *testing.T) {
	script := filepath.Join(t.TempDir(), "oc")
	content := "#!/bin/sh\n[ \"$1 $2 $3 $4 $5\" = \"get namespaces -o name --no-headers\" ] || exit 1\nprintf '%s\\n' namespace/mathwizz-test-github namespace/mathwizz-test-github-20261005b\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}

	names, err := (OCNamespaceNameLister{Command: script}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mathwizz-test-github", "mathwizz-test-github-20261005b"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
}

type staticNameLister struct {
	names []string
}

func (l staticNameLister) List(context.Context) ([]string, error) {
	return append([]string(nil), l.names...), nil
}

func namespace(name, runID string, owned bool) *unstructured.Unstructured {
	labels := map[string]any{RunIDLabel: runID}
	if owned {
		labels[ManagedByLabel] = ManagedByValue
	}
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": name, "labels": labels}}}
}
