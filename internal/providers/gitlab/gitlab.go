package gitlab

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	e2e "github.com/konflux-ci/e2e-tests/pkg/clients/gitlab"
	"github.com/redhat-appstudio/konflux-test/internal/providers"
	gl "github.com/xanzy/go-gitlab"
)

type client interface {
	GetAllProjects() ([]*gl.Project, error)
	GetFileMetaData(string, string, string) (*gl.File, error)
	GetFile(string, string, string) (string, error)
	UpdateFile(string, string, string, string) (string, error)
	ForkRepository(string, string, string, string) (*gl.Project, error)
}

type Adapter struct{ client client }

func New(client client) *Adapter { return &Adapter{client: client} }

func NewFromEnv() (*Adapter, error) {
	return NewFromCredentials(os.Getenv("GITLAB_BOT_TOKEN"), os.Getenv("GITLAB_API_URL"), os.Getenv("GITLAB_GROUP_ID"))
}

func NewFromCredentials(token, apiURL, groupID string) (*Adapter, error) {
	if apiURL == "" {
		apiURL = "https://gitlab.com/api/v4"
	}
	client, err := e2e.NewGitlabClient(token, apiURL, groupID)
	if err != nil {
		return nil, err
	}
	return New(client), nil
}

func (a *Adapter) EnsureFork(_ context.Context, source providers.SourceRepository, fixture providers.FixtureRepository) (providers.FixtureRepository, error) {
	if a == nil || a.client == nil {
		return providers.FixtureRepository{}, fmt.Errorf("gitlab client is required")
	}
	if fixture.Owner == "" {
		fixture.Owner = source.Owner
	}
	if fixture.Name == "" {
		fixture.Name = source.Name + "-konflux-test"
	}
	projects, err := a.client.GetAllProjects()
	if err != nil {
		return providers.FixtureRepository{}, err
	}
	for _, project := range projects {
		if project != nil && project.PathWithNamespace == fixture.Owner+"/"+fixture.Name {
			return a.withURL(fixture, project), nil
		}
	}
	fork, err := a.client.ForkRepository(source.Owner, source.Name, fixture.Owner, fixture.Name)
	if err != nil {
		return providers.FixtureRepository{}, fmt.Errorf("fork gitlab repository: %w", err)
	}
	return a.withURL(fixture, fork), nil
}

func (a *Adapter) VerifyFork(_ context.Context, fixture providers.FixtureRepository) error {
	projects, err := a.client.GetAllProjects()
	if err != nil {
		return err
	}
	wanted := fixture.Owner + "/" + fixture.Name
	for _, project := range projects {
		if project != nil && project.PathWithNamespace == wanted {
			return nil
		}
	}
	return fmt.Errorf("gitlab fork %s is not available", wanted)
}

func (a *Adapter) ReadFile(_ context.Context, fixture providers.FixtureRepository, path, branch string) (providers.FileVersion, error) {
	project := fixture.Owner + "/" + fixture.Name
	metadata, err := a.client.GetFileMetaData(project, path, branch)
	if err != nil {
		return providers.FileVersion{}, err
	}
	content, err := a.client.GetFile(project, path, branch)
	if err != nil {
		return providers.FileVersion{}, err
	}
	sha := metadata.CommitID
	if sha == "" {
		sha = metadata.LastCommitID
	}
	return providers.FileVersion{Path: path, Branch: branch, Content: content, SHA: sha, URL: metadata.FilePath}, nil
}

func (a *Adapter) UpdateFile(_ context.Context, fixture providers.FixtureRepository, path, branch, content, expectedSHA string) (providers.Commit, error) {
	project := fixture.Owner + "/" + fixture.Name
	metadata, err := a.client.GetFileMetaData(project, path, branch)
	if err != nil {
		return providers.Commit{}, err
	}
	actual := metadata.CommitID
	if actual == "" {
		actual = metadata.LastCommitID
	}
	if err := providers.ValidateExpectedSHA(expectedSHA, actual); err != nil {
		return providers.Commit{}, err
	}
	sha, err := a.client.UpdateFile(project, path, content, branch)
	if err != nil {
		return providers.Commit{}, err
	}
	return providers.Commit{SHA: sha, Message: "konflux-test trigger", CreatedAt: time.Now().UTC()}, nil
}

func (a *Adapter) CollectCommitEvidence(_ context.Context, fixture providers.FixtureRepository, commit providers.Commit) (map[string]any, error) {
	return map[string]any{"provider": "gitlab", "repository": fixture.URL, "owner": fixture.Owner, "name": fixture.Name, "commitSHA": commit.SHA}, nil
}

func (a *Adapter) withURL(fixture providers.FixtureRepository, project *gl.Project) providers.FixtureRepository {
	if project != nil {
		if project.Path != "" {
			fixture.Name = project.Path
		}
		if project.Namespace != nil && project.Namespace.FullPath != "" {
			fixture.Owner = project.Namespace.FullPath
		}
		if project.WebURL != "" {
			fixture.URL = project.WebURL
		}
	}
	if fixture.URL == "" {
		fixture.URL = fmt.Sprintf("https://gitlab.com/%s/%s", strings.Trim(fixture.Owner, "/"), fixture.Name)
	}
	return fixture
}
