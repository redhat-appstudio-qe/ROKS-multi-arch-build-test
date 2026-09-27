package gitlab

import (
	"testing"

	"github.com/redhat-appstudio/konflux-test/internal/providers"
	gl "github.com/xanzy/go-gitlab"
)

type fakeClient struct{ projects []*gl.Project }

func (f *fakeClient) GetAllProjects() ([]*gl.Project, error) { return f.projects, nil }
func (f *fakeClient) GetFileMetaData(string, string, string) (*gl.File, error) {
	return &gl.File{CommitID: "sha-1"}, nil
}
func (f *fakeClient) GetFile(string, string, string) (string, error)            { return "FROM base", nil }
func (f *fakeClient) UpdateFile(string, string, string, string) (string, error) { return "sha-2", nil }
func (f *fakeClient) ForkRepository(string, string, string, string) (*gl.Project, error) {
	return &gl.Project{Path: "fork", PathWithNamespace: "org/fork", WebURL: "https://gitlab.com/org/fork"}, nil
}

func TestEnsureForkReusesExistingProject(t *testing.T) {
	adapter := New(&fakeClient{projects: []*gl.Project{{PathWithNamespace: "org/fixture"}}})
	got, err := adapter.EnsureFork(nil, providers.SourceRepository{Owner: "source", Name: "repo"}, providers.FixtureRepository{Owner: "org", Name: "fixture"})
	if err != nil || got.Name != "fixture" {
		t.Fatalf("got %#v, %v", got, err)
	}
}
