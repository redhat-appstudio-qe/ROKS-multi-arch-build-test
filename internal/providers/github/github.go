package github

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	gh "github.com/google/go-github/v66/github"
	e2e "github.com/konflux-ci/e2e-tests/pkg/clients/github"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
)

type client interface {
	CheckIfRepositoryExist(string) bool
	GetFile(string, string, string) (*gh.RepositoryContent, error)
	UpdateFile(string, string, string, string, string) (*gh.RepositoryContentResponse, error)
	ForkRepositoryFromOrg(string, string, string) (*gh.Repository, error)
}

type Adapter struct {
	client       client
	organization string
}

func New(client client, organization string) *Adapter {
	return &Adapter{client: client, organization: organization}
}

func NewFromEnv() (*Adapter, error) {
	return NewFromCredentials(os.Getenv("GITHUB_TOKEN"), os.Getenv("MY_GITHUB_ORG"))
}

func NewFromCredentials(token, organization string) (*Adapter, error) {
	if strings.TrimSpace(organization) == "" {
		return nil, fmt.Errorf("MY_GITHUB_ORG is required")
	}
	client, err := e2e.NewGithubClient(token, organization)
	if err != nil {
		return nil, err
	}
	return New(client, organization), nil
}

func (a *Adapter) EnsureFork(_ context.Context, source providers.SourceRepository, fixture providers.FixtureRepository) (providers.FixtureRepository, error) {
	if a == nil || a.client == nil {
		return providers.FixtureRepository{}, fmt.Errorf("github client is required")
	}
	if fixture.Owner == "" {
		fixture.Owner = a.organization
	}
	if fixture.Name == "" {
		fixture.Name = source.Name + "-konflux-test"
	}
	if a.client.CheckIfRepositoryExist(fixture.Name) {
		return a.withURL(fixture), nil
	}
	fork, err := a.client.ForkRepositoryFromOrg(source.Name, fixture.Name, source.Owner)
	if err != nil {
		return providers.FixtureRepository{}, fmt.Errorf("fork github repository: %w", err)
	}
	fixture.Name = fork.GetName()
	fixture.Owner = fork.GetOwner().GetLogin()
	fixture.URL = fork.GetHTMLURL()
	return fixture, nil
}

func (a *Adapter) VerifyFork(_ context.Context, fixture providers.FixtureRepository) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("github client is required")
	}
	if fixture.Name == "" || !a.client.CheckIfRepositoryExist(fixture.Name) {
		return fmt.Errorf("github fork %s/%s is not available", fixture.Owner, fixture.Name)
	}
	return nil
}

func (a *Adapter) ReadFile(_ context.Context, fixture providers.FixtureRepository, path, branch string) (providers.FileVersion, error) {
	file, err := a.client.GetFile(fixture.Name, path, branch)
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
	updated, err := a.client.UpdateFile(fixture.Name, path, content, branch, expectedSHA)
	if err != nil {
		return providers.Commit{}, err
	}
	return providers.Commit{SHA: updated.Commit.GetSHA(), URL: updated.Commit.GetHTMLURL(), Message: "konflux-test trigger", CreatedAt: time.Now().UTC()}, nil
}

func (a *Adapter) CollectCommitEvidence(_ context.Context, fixture providers.FixtureRepository, commit providers.Commit) (map[string]any, error) {
	return map[string]any{"provider": "github", "repository": fixture.URL, "owner": fixture.Owner, "name": fixture.Name, "commitSHA": commit.SHA, "commitURL": commit.URL}, nil
}

func (a *Adapter) withURL(fixture providers.FixtureRepository) providers.FixtureRepository {
	if fixture.URL == "" {
		fixture.URL = fmt.Sprintf("https://github.com/%s/%s", fixture.Owner, fixture.Name)
	}
	return fixture
}
