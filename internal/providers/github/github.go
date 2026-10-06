package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gh "github.com/google/go-github/v66/github"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
)

const (
	CanonicalFixtureURL = "https://github.com/redhat-appstudio-qe/dr_test_mathwizz"
	canonicalOwner      = "redhat-appstudio-qe"
	canonicalName       = "dr_test_mathwizz"
)

type client interface {
	GetRepository(string, string) (*gh.Repository, error)
	GetFile(string, string, string, string) (*gh.RepositoryContent, error)
	UpdateFile(string, string, string, string, string, string) (*gh.RepositoryContentResponse, error)
}

type apiClient struct{ client *gh.Client }

func (c apiClient) GetRepository(owner, name string) (*gh.Repository, error) {
	repository, _, err := c.client.Repositories.Get(context.Background(), owner, name)
	return repository, err
}

func (c apiClient) GetFile(owner, name, path, branch string) (*gh.RepositoryContent, error) {
	file, _, _, err := c.client.Repositories.GetContents(context.Background(), owner, name, path, &gh.RepositoryContentGetOptions{Ref: branch})
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("github path %s is a directory", path)
	}
	return file, nil
}

func (c apiClient) UpdateFile(owner, name, path, content, branch, sha string) (*gh.RepositoryContentResponse, error) {
	message := "konflux-test trigger"
	response, _, err := c.client.Repositories.UpdateFile(context.Background(), owner, name, path, &gh.RepositoryContentFileOptions{Message: &message, Content: []byte(content), Branch: &branch, SHA: &sha})
	return response, err
}

type Adapter struct {
	client client
	retry  providers.RetryPolicy
}

func New(client client) *Adapter {
	return NewWithRetry(client, providers.DefaultRetryPolicy())
}

func NewWithRetry(client client, retry providers.RetryPolicy) *Adapter {
	return &Adapter{client: client, retry: retry}
}

func NewFromEnv() (*Adapter, error) { return NewFromCredentials(os.Getenv("GITHUB_TOKEN")) }

func NewFromCredentials(token string) (*Adapter, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is required")
	}
	return New(apiClient{client: gh.NewClient(nil).WithAuthToken(token)}), nil
}

func (a *Adapter) ValidateFixture(ctx context.Context, fixture providers.FixtureRepository) (providers.FixtureRepository, error) {
	if a == nil || a.client == nil {
		return providers.FixtureRepository{}, fmt.Errorf("github client is required")
	}
	if fixture.Owner != canonicalOwner || fixture.Name != canonicalName || (fixture.URL != "" && fixture.URL != CanonicalFixtureURL) {
		return providers.FixtureRepository{}, fmt.Errorf("github fixture must be %s", CanonicalFixtureURL)
	}
	var repository *gh.Repository
	err := providers.RetryOn5xx(ctx, a.retry, func() error {
		var err error
		repository, err = a.client.GetRepository(canonicalOwner, canonicalName)
		return err
	}, isGitHub5xx)
	if err != nil {
		return providers.FixtureRepository{}, fmt.Errorf("validate github fixture: %w", err)
	}
	if repository.GetOwner().GetLogin() != canonicalOwner || repository.GetName() != canonicalName {
		return providers.FixtureRepository{}, fmt.Errorf("github API returned unexpected fixture identity")
	}
	return providers.FixtureRepository{Owner: canonicalOwner, Name: canonicalName, URL: CanonicalFixtureURL}, nil
}

func (a *Adapter) ReadFile(ctx context.Context, fixture providers.FixtureRepository, path, branch string) (providers.FileVersion, error) {
	var file *gh.RepositoryContent
	err := providers.RetryOn5xx(ctx, a.retry, func() error {
		var err error
		file, err = a.client.GetFile(fixture.Owner, fixture.Name, path, branch)
		return err
	}, isGitHub5xx)
	if err != nil {
		return providers.FileVersion{}, err
	}
	content, err := file.GetContent()
	if err != nil {
		return providers.FileVersion{}, fmt.Errorf("decode github file %s: %w", path, err)
	}
	return providers.FileVersion{Path: path, Branch: branch, Content: content, SHA: file.GetSHA(), URL: file.GetHTMLURL()}, nil
}

func (a *Adapter) UpdateFile(ctx context.Context, fixture providers.FixtureRepository, path, branch, content, expectedSHA string) (providers.Commit, error) {
	current, err := a.ReadFile(ctx, fixture, path, branch)
	if err != nil {
		return providers.Commit{}, err
	}
	if err := providers.ValidateExpectedSHA(expectedSHA, current.SHA); err != nil {
		return providers.Commit{}, err
	}
	var updated *gh.RepositoryContentResponse
	err = providers.RetryOn5xx(ctx, a.retry, func() error {
		var err error
		updated, err = a.client.UpdateFile(fixture.Owner, fixture.Name, path, content, branch, expectedSHA)
		return err
	}, isGitHub5xx)
	if err != nil {
		return providers.Commit{}, err
	}
	return providers.Commit{SHA: updated.Commit.GetSHA(), URL: updated.Commit.GetHTMLURL(), Message: "konflux-test trigger", CreatedAt: time.Now().UTC()}, nil
}

func isGitHub5xx(err error) bool {
	var response *gh.ErrorResponse
	return errors.As(err, &response) && response.Response != nil && response.Response.StatusCode >= 500 && response.Response.StatusCode <= 599
}
