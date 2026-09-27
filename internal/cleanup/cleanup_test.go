package cleanup

import (
	"context"
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/evidence"
	"github.com/redhat-appstudio/konflux-test/internal/model"
)

type fakeClient struct {
	resources []Resource
	deleted   []Resource
}

func (f *fakeClient) List(context.Context, string) ([]Resource, error) { return f.resources, nil }
func (f *fakeClient) Get(_ context.Context, resource Resource) (Resource, error) {
	return resource, nil
}
func (f *fakeClient) Delete(_ context.Context, resource Resource) error {
	f.deleted = append(f.deleted, resource)
	return nil
}

func TestCleanupDeletesOnlyOwnedTransientResources(t *testing.T) {
	store := evidence.NewManifestStore(t.TempDir())
	manifest := model.RunManifest{RunID: "run-1", TargetClusterServer: "https://api.example", Fixture: model.FixtureIdentity{TenantNamespace: "tenant", Application: "app"}}
	if err := store.Create(manifest); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{resources: []Resource{
		{Kind: "TaskRun", Namespace: "tenant", Name: "owned", Labels: map[string]string{ManagedByLabel: ManagedByValue, RunIDLabel: "run-1"}},
		{Kind: "PipelineRun", Namespace: "tenant", Name: "acceptance", Labels: map[string]string{ManagedByLabel: ManagedByValue, RunIDLabel: "run-1"}},
		{Kind: "ConfigMap", Namespace: "tenant", Name: "unowned", Labels: map[string]string{}},
	}}
	result, err := (Service{Store: store, Client: client}).Cleanup(context.Background(), "run-1", "https://api.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deleted) != 1 || len(result.Refused) != 2 || len(client.deleted) != 1 {
		t.Fatalf("result=%#v deleted=%#v", result, client.deleted)
	}
}
