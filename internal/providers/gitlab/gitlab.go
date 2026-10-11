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
	// GetProject looks up a project by its full namespace and name path.
	GetProject(string) (*gl.Project, error)
	// GetFileMetaData fetches file version metadata for a branch and path.
	GetFileMetaData(string, string, string) (*gl.File, error)
	// GetFile fetches the encoded file content for a branch and path.
	GetFile(string, string, string) (string, error)
	// UpdateFile commits replacement content using the current file SHA.
	UpdateFile(string, string, string, string, string, string) error
}

type apiClient struct{ client *gl.Client }

// GetProject fetches a project from the GitLab API.
func (c apiClient) GetProject(project string) (*gl.Project, error) {
	result, _, err := c.client.Projects.GetProject(project, nil)
	return result, err
}

// GetFileMetaData fetches GitLab metadata for a file on the requested branch.
func (c apiClient) GetFileMetaData(project, path, branch string) (*gl.File, error) {
	result, _, err := c.client.RepositoryFiles.GetFileMetaData(project, path, &gl.GetFileMetaDataOptions{Ref: &branch})
	return result, err
}

// GetFile fetches encoded file content from the requested GitLab branch.
func (c apiClient) GetFile(project, path, branch string) (string, error) {
	result, _, err := c.client.RepositoryFiles.GetFile(project, path, &gl.GetFileOptions{Ref: &branch})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// UpdateFile commits file content to GitLab using the supplied last commit ID.
func (c apiClient) UpdateFile(project, path, content, branch, expectedSHA, message string) error {
	_, _, err := c.client.RepositoryFiles.UpdateFile(project, path, &gl.UpdateFileOptions{Branch: &branch, Content: &content, LastCommitID: &expectedSHA, CommitMessage: &message})
	return err
}

type Adapter struct {
	client client
	retry  providers.RetryPolicy
}

// New creates a GitLab adapter with the default retry policy.
func New(client client) *Adapter {
	return NewWithRetry(client, providers.DefaultRetryPolicy())
}

// NewWithRetry creates a GitLab adapter with the supplied retry policy.
func NewWithRetry(client client, retry providers.RetryPolicy) *Adapter {
	return &Adapter{client: client, retry: retry}
}

// NewFromEnv creates a GitLab adapter using GITLAB_BOT_TOKEN and GITLAB_API_URL.
func NewFromEnv() (*Adapter, error) {
	return NewFromCredentials(os.Getenv("GITLAB_BOT_TOKEN"), os.Getenv("GITLAB_API_URL"))
}

// NewFromCredentials validates a token and creates a GitLab adapter for apiURL.
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

// ValidateFixture confirms the project is the canonical GitLab test fixture.
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

// ReadFile returns decoded file content and version metadata from GitLab.
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

// UpdateFile checks the current SHA, commits replacement content, and returns updated version metadata.
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

// isGitLab5xx reports whether err contains an HTTP 5xx GitLab response.
func isGitLab5xx(err error) bool {
	var response *gl.ErrorResponse
	return errors.As(err, &response) && response.Response != nil && response.Response.StatusCode >= 500 && response.Response.StatusCode <= 599
}
