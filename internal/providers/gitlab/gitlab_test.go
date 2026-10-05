package gitlab

import (
	"context"
	"errors"
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/providers"
	gl "github.com/xanzy/go-gitlab"
)

type fakeClient struct {
	project     *gl.Project
	getCalls    int
	updateCalls int
}

func (f *fakeClient) GetProject(string) (*gl.Project, error) {
	f.getCalls++
	if f.project == nil {
		return nil, errors.New("not found")
	}
	return f.project, nil
}

func (f *fakeClient) GetFileMetaData(string, string, string) (*gl.File, error) {
	return &gl.File{CommitID: "sha-1"}, nil
}

func (f *fakeClient) GetFile(string, string, string) (string, error) { return "FROM base", nil }

func (f *fakeClient) UpdateFile(string, string, string, string, string, string) (string, error) {
	f.updateCalls++
	return "sha-2", nil
}

func TestValidateFixtureReusesCanonicalProject(t *testing.T) {
	client := &fakeClient{project: &gl.Project{Path: "dr_test_mathwizz_gl", PathWithNamespace: "konflux-qe/dr_test_mathwizz_gl", WebURL: CanonicalFixtureURL}}
	got, err := New(client).ValidateFixture(context.Background(), providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl"})
	if err != nil {
		t.Fatal(err)
	}
	want := providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl", URL: CanonicalFixtureURL}
	if got != want {
		t.Fatalf("fixture = %#v, want %#v", got, want)
	}
	if client.getCalls != 1 {
		t.Fatalf("project lookup calls = %d", client.getCalls)
	}
}

func TestValidateFixtureRejectsNonCanonicalProjectWithoutCreation(t *testing.T) {
	client := &fakeClient{}
	_, err := New(client).ValidateFixture(context.Background(), providers.FixtureRepository{Owner: "other", Name: "dr_test_mathwizz_gl"})
	if err == nil {
		t.Fatal("expected canonical fixture rejection")
	}
	if client.getCalls != 0 || client.updateCalls != 0 {
		t.Fatalf("client calls = get %d update %d", client.getCalls, client.updateCalls)
	}
}

func TestReadAndUpdateFileUseCanonicalProject(t *testing.T) {
	client := &fakeClient{}
	adapter := New(client)
	fixture := providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl", URL: CanonicalFixtureURL}
	file, err := adapter.ReadFile(context.Background(), fixture, "web-server/Dockerfile", "main")
	if err != nil || file.SHA != "sha-1" {
		t.Fatalf("file = %#v, err = %v", file, err)
	}
	if _, err := adapter.UpdateFile(context.Background(), fixture, "web-server/Dockerfile", "main", "content", "sha-1"); err != nil {
		t.Fatal(err)
	}
	if client.updateCalls != 1 {
		t.Fatalf("update calls = %d", client.updateCalls)
	}
}
