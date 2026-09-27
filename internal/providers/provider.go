package providers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type SourceRepository struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type FixtureRepository struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type FileVersion struct {
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Content string `json:"content"`
	SHA     string `json:"sha"`
	URL     string `json:"url,omitempty"`
}

type Commit struct {
	SHA       string    `json:"sha"`
	URL       string    `json:"url,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

type Provider interface {
	EnsureFork(context.Context, SourceRepository, FixtureRepository) (FixtureRepository, error)
	VerifyFork(context.Context, FixtureRepository) error
	ReadFile(context.Context, FixtureRepository, string, string) (FileVersion, error)
	UpdateFile(context.Context, FixtureRepository, string, string, string, string) (Commit, error)
	CollectCommitEvidence(context.Context, FixtureRepository, Commit) (map[string]any, error)
}

func ParseRepository(raw string) (SourceRepository, error) {
	if strings.TrimSpace(raw) == "" {
		return SourceRepository{}, fmt.Errorf("repository is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return SourceRepository{}, fmt.Errorf("invalid repository URL %q", raw)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return SourceRepository{}, fmt.Errorf("repository URL %q has no owner/name", raw)
	}
	return SourceRepository{Owner: parts[len(parts)-2], Name: strings.TrimSuffix(parts[len(parts)-1], ".git"), URL: raw}, nil
}

func ValidateExpectedSHA(expected, actual string) error {
	if strings.TrimSpace(expected) == "" {
		return fmt.Errorf("expected file version is required")
	}
	if expected != actual {
		return fmt.Errorf("stale file version: expected %q, got %q", expected, actual)
	}
	return nil
}
