package github

import (
	"testing"

	gh "github.com/google/go-github/v66/github"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
)

type fakeClient struct{ exists bool }

func (f *fakeClient) CheckIfRepositoryExist(string) bool { return f.exists }
func (f *fakeClient) GetFile(string, string, string) (*gh.RepositoryContent, error) {
	return &gh.RepositoryContent{SHA: gh.String("sha-1"), Content: gh.String("ZmlsZQ==")}, nil
}
func (f *fakeClient) UpdateFile(string, string, string, string, string) (*gh.RepositoryContentResponse, error) {
	return &gh.RepositoryContentResponse{Commit: gh.Commit{SHA: gh.String("commit-1")}}, nil
}
func (f *fakeClient) ForkRepositoryFromOrg(string, string, string) (*gh.Repository, error) {
	return &gh.Repository{Name: gh.String("fork"), HTMLURL: gh.String("https://github.com/org/fork"), Owner: &gh.User{Login: gh.String("org")}}, nil
}

func TestEnsureForkReusesExistingRepository(t *testing.T) {
	adapter := New(&fakeClient{exists: true}, "org")
	got, err := adapter.EnsureFork(nil, providers.SourceRepository{Owner: "source", Name: "repo"}, providers.FixtureRepository{Name: "fixture"})
	if err != nil || got.Name != "fixture" {
		t.Fatalf("got %#v, %v", got, err)
	}
}
