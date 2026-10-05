package gitlab

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/providers"
	gl "github.com/xanzy/go-gitlab"
)

const (
	CanonicalFixtureURL = "https://gitlab.com/konflux-qe/dr_test_mathwizz_gl"
	canonicalProject    = "konflux-qe/dr_test_mathwizz_gl"
)

type client interface {
	GetProject(string) (*gl.Project, error)
	GetFileMetaData(string, string, string) (*gl.File, error)
	GetFile(string, string, string) (string, error)
	UpdateFile(string, string, string, string, string, string) (string, error)
}

type apiClient struct{ client *gl.Client }

func (c apiClient) GetProject(project string) (*gl.Project, error) {
	result, _, err := c.client.Projects.GetProject(project, nil)
	return result, err
}

func (c apiClient) GetFileMetaData(project, path, branch string) (*gl.File, error) {
	result, _, err := c.client.RepositoryFiles.GetFileMetaData(project, path, &gl.GetFileMetaDataOptions{Ref: &branch})
	return result, err
}

func (c apiClient) GetFile(project, path, branch string) (string, error) {
	result, _, err := c.client.RepositoryFiles.GetFile(project, path, &gl.GetFileOptions{Ref: &branch})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func (c apiClient) UpdateFile(project, path, content, branch, expectedSHA, message string) (string, error) {
	_, _, err := c.client.RepositoryFiles.UpdateFile(project, path, &gl.UpdateFileOptions{Branch: &branch, Content: &content, LastCommitID: &expectedSHA, CommitMessage: &message})
	if err != nil {
		return "", err
	}
	metadata, _, err := c.client.RepositoryFiles.GetFileMetaData(project, path, &gl.GetFileMetaDataOptions{Ref: &branch})
	if err != nil {
		return "", err
	}
	if metadata.CommitID != "" {
		return metadata.CommitID, nil
	}
	return metadata.LastCommitID, nil
}

type Adapter struct{ client client }

func New(client client) *Adapter { return &Adapter{client: client} }

func NewFromEnv() (*Adapter, error) {
	return NewFromCredentials(os.Getenv("GITLAB_BOT_TOKEN"), os.Getenv("GITLAB_API_URL"))
}

func NewFromCredentials(token, apiURL string) (*Adapter, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("GITLAB_BOT_TOKEN is required")
	}
	if strings.TrimSpace(apiURL) == "" {
		apiURL = "https://gitlab.com/api/v4"
	}
	gitlabClient, err := gl.NewClient(token, gl.WithBaseURL(apiURL))
	if err != nil {
		return nil, err
	}
	return New(apiClient{client: gitlabClient}), nil
}

func (a *Adapter) ValidateFixture(ctx context.Context, fixture providers.FixtureRepository) (providers.FixtureRepository, error) {
	if a == nil || a.client == nil {
		return providers.FixtureRepository{}, fmt.Errorf("gitlab client is required")
	}
	if fixture.Owner+"/"+fixture.Name != canonicalProject || (fixture.URL != "" && fixture.URL != CanonicalFixtureURL) {
		return providers.FixtureRepository{}, fmt.Errorf("gitlab fixture must be %s", CanonicalFixtureURL)
	}
	project, err := a.client.GetProject(canonicalProject)
	if err != nil {
		return providers.FixtureRepository{}, fmt.Errorf("validate gitlab fixture: %w", err)
	}
	if project.PathWithNamespace != canonicalProject {
		return providers.FixtureRepository{}, fmt.Errorf("gitlab API returned unexpected fixture identity")
	}
	return providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl", URL: CanonicalFixtureURL}, nil
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

func (a *Adapter) UpdateFile(ctx context.Context, fixture providers.FixtureRepository, path, branch, content, expectedSHA string) (providers.Commit, error) {
	current, err := a.ReadFile(ctx, fixture, path, branch)
	if err != nil {
		return providers.Commit{}, err
	}
	if err := providers.ValidateExpectedSHA(expectedSHA, current.SHA); err != nil {
		return providers.Commit{}, err
	}
	project := fixture.Owner + "/" + fixture.Name
	sha, err := a.client.UpdateFile(project, path, content, branch, expectedSHA, "konflux-test trigger")
	if err != nil {
		return providers.Commit{}, err
	}
	return providers.Commit{SHA: sha, Message: "konflux-test trigger", CreatedAt: time.Now().UTC()}, nil
}
