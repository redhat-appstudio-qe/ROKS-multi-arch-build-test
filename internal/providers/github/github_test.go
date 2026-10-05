package github

import (
	"context"
	"errors"
	"testing"

	gh "github.com/google/go-github/v66/github"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
)

type fakeClient struct {
	repository  *gh.Repository
	getCalls    int
	updateCalls int
}

func (f *fakeClient) GetRepository(string, string) (*gh.Repository, error) {
	f.getCalls++
	if f.repository == nil {
		return nil, errors.New("not found")
	}
	return f.repository, nil
}

func (f *fakeClient) GetFile(string, string, string, string) (*gh.RepositoryContent, error) {
	return &gh.RepositoryContent{SHA: gh.String("sha-1"), Content: gh.String("ZmlsZQ==")}, nil
}

func (f *fakeClient) UpdateFile(string, string, string, string, string, string) (*gh.RepositoryContentResponse, error) {
	f.updateCalls++
	return &gh.RepositoryContentResponse{Commit: gh.Commit{SHA: gh.String("commit-1")}}, nil
}

func TestValidateFixtureReusesCanonicalRepository(t *testing.T) {
	client := &fakeClient{repository: &gh.Repository{Name: gh.String("dr_test_mathwizz"), Owner: &gh.User{Login: gh.String("redhat-appstudio-qe")}}}
	got, err := New(client).ValidateFixture(context.Background(), providers.FixtureRepository{Owner: "redhat-appstudio-qe", Name: "dr_test_mathwizz"})
	if err != nil {
		t.Fatal(err)
	}
	want := providers.FixtureRepository{Owner: "redhat-appstudio-qe", Name: "dr_test_mathwizz", URL: CanonicalFixtureURL}
	if got != want {
		t.Fatalf("fixture = %#v, want %#v", got, want)
	}
	if client.getCalls != 1 {
		t.Fatalf("repository lookup calls = %d", client.getCalls)
	}
}

func TestValidateFixtureRejectsNonCanonicalRepositoryWithoutCreation(t *testing.T) {
	client := &fakeClient{}
	_, err := New(client).ValidateFixture(context.Background(), providers.FixtureRepository{Owner: "other", Name: "dr_test_mathwizz"})
	if err == nil {
		t.Fatal("expected canonical fixture rejection")
	}
	if client.getCalls != 0 || client.updateCalls != 0 {
		t.Fatalf("client calls = get %d update %d", client.getCalls, client.updateCalls)
	}
}

func TestReadAndUpdateFileUseCanonicalRepository(t *testing.T) {
	client := &fakeClient{}
	adapter := New(client)
	fixture := providers.FixtureRepository{Owner: "redhat-appstudio-qe", Name: "dr_test_mathwizz", URL: CanonicalFixtureURL}
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
