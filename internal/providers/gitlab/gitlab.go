package gitlab

import (
	"context"
	"encoding/base64"
	"errors"
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
	UpdateFile(string, string, string, string, string, string) error
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

func (c apiClient) UpdateFile(project, path, content, branch, expectedSHA, message string) error {
	_, _, err := c.client.RepositoryFiles.UpdateFile(project, path, &gl.UpdateFileOptions{Branch: &branch, Content: &content, LastCommitID: &expectedSHA, CommitMessage: &message})
	return err
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
	var project *gl.Project
	err := providers.RetryOn5xx(ctx, a.retry, func() error {
		var err error
		project, err = a.client.GetProject(canonicalProject)
		return err
	}, isGitLab5xx)
	if err != nil {
		return providers.FixtureRepository{}, fmt.Errorf("validate gitlab fixture: %w", err)
	}
	if project.PathWithNamespace != canonicalProject {
		return providers.FixtureRepository{}, fmt.Errorf("gitlab API returned unexpected fixture identity")
	}
	return providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl", URL: CanonicalFixtureURL}, nil
}

func (a *Adapter) ReadFile(ctx context.Context, fixture providers.FixtureRepository, path, branch string) (providers.FileVersion, error) {
	project := fixture.Owner + "/" + fixture.Name
	var metadata *gl.File
	err := providers.RetryOn5xx(ctx, a.retry, func() error {
		var err error
		metadata, err = a.client.GetFileMetaData(project, path, branch)
		return err
	}, isGitLab5xx)
	if err != nil {
		return providers.FileVersion{}, err
	}
	var content string
	err = providers.RetryOn5xx(ctx, a.retry, func() error {
		var err error
		content, err = a.client.GetFile(project, path, branch)
		return err
	}, isGitLab5xx)
	if err != nil {
		return providers.FileVersion{}, err
	}
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return providers.FileVersion{}, fmt.Errorf("decode GitLab file %s: %w", path, err)
	}
	sha := metadata.CommitID
	if sha == "" {
		sha = metadata.LastCommitID
	}
	return providers.FileVersion{Path: path, Branch: branch, Content: string(decoded), SHA: sha, URL: metadata.FilePath}, nil
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
	err = providers.RetryOn5xx(ctx, a.retry, func() error {
		return a.client.UpdateFile(project, path, content, branch, expectedSHA, "konflux-test trigger")
	}, isGitLab5xx)
	if err != nil {
		return providers.Commit{}, err
	}
	var metadata *gl.File
	err = providers.RetryOn5xx(ctx, a.retry, func() error {
		var err error
		metadata, err = a.client.GetFileMetaData(project, path, branch)
		return err
	}, isGitLab5xx)
	if err != nil {
		return providers.Commit{}, err
	}
	sha := metadata.CommitID
	if sha == "" {
		sha = metadata.LastCommitID
	}
	return providers.Commit{SHA: sha, Message: "konflux-test trigger", CreatedAt: time.Now().UTC()}, nil
}

func isGitLab5xx(err error) bool {
	var response *gl.ErrorResponse
	return errors.As(err, &response) && response.Response != nil && response.Response.StatusCode >= 500 && response.Response.StatusCode <= 599
}
