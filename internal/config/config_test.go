package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseRunCommand(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	t.Setenv("MY_GITHUB_ORG", "org")
	got, err := Parse([]string{"run", "github", "--cluster-server", "https://api.example:6443"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != CommandRun || got.Provider != ProviderGitHub || got.ClusterServer != "https://api.example:6443" {
		t.Fatalf("unexpected command: %#v", got)
	}
	if got.SourceRepository != DefaultGitHubRepo || got.SourceBranch != DefaultBranch {
		t.Fatalf("unexpected defaults: %#v", got)
	}
	if got.TenantNamespace != DefaultGitHubTenantNamespace {
		t.Fatalf("tenant namespace = %q, want %q", got.TenantNamespace, DefaultGitHubTenantNamespace)
	}
}

func TestParseUsesProviderSpecificTenantNamespaces(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "gh-token")
	t.Setenv("MY_GITHUB_ORG", "gh-org")
	t.Setenv("GITLAB_BOT_TOKEN", "gl-token")
	t.Setenv("GITLAB_GROUP_ID", "42")

	githubRequest, err := Parse([]string{"run", "github", "--cluster-server", "https://api.example"})
	if err != nil {
		t.Fatal(err)
	}
	gitlabRequest, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example"})
	if err != nil {
		t.Fatal(err)
	}
	if githubRequest.TenantNamespace == gitlabRequest.TenantNamespace {
		t.Fatalf("providers share tenant namespace %q", githubRequest.TenantNamespace)
	}
	if githubRequest.TenantNamespace != DefaultGitHubTenantNamespace || gitlabRequest.TenantNamespace != DefaultGitLabTenantNamespace {
		t.Fatalf("unexpected provider namespaces: github=%q gitlab=%q", githubRequest.TenantNamespace, gitlabRequest.TenantNamespace)
	}
}

func TestParseHonorsExplicitTenantNamespace(t *testing.T) {
	t.Setenv("GITLAB_BOT_TOKEN", "token")
	t.Setenv("GITLAB_GROUP_ID", "42")
	got, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example", "--tenant-namespace", "custom-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantNamespace != "custom-tenant" {
		t.Fatalf("tenant namespace = %q, want custom-tenant", got.TenantNamespace)
	}
}

func TestParseLoadsCredentialsFromEnvFile(t *testing.T) {
	envFile := t.TempDir() + "/konflux.env"
	if err := os.WriteFile(envFile, []byte("GITLAB_BOT_TOKEN='file-token'\nGITLAB_GROUP_ID=42\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITLAB_BOT_TOKEN", "")
	t.Setenv("GITLAB_GROUP_ID", "")
	got, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example", "--env-file", envFile})
	if err != nil {
		t.Fatal(err)
	}
	if got.Credentials.GitLabToken != "file-token" || got.Credentials.GitLabGroupID != "42" {
		t.Fatalf("credentials = %#v", got.Credentials)
	}
}

func TestParseReportsMissingProviderCredentialsTogether(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("MY_GITHUB_ORG", "")
	_, err := Parse([]string{"run", "github", "--cluster-server", "https://api.example"})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") || !strings.Contains(err.Error(), "MY_GITHUB_ORG") {
		t.Fatalf("error = %v, want both missing credentials", err)
	}
}

func TestParseCollectAndCleanupRequireRunID(t *testing.T) {
	for _, command := range []string{"collect-logs", "cleanup"} {
		if _, err := Parse([]string{command, "--cluster-server", "https://api.example"}); err == nil {
			t.Errorf("%s accepted a missing run ID", command)
		}
		if got, err := Parse([]string{command, "run-123", "--cluster-server", "https://api.example"}); err != nil || got.RunID != "run-123" {
			t.Errorf("%s parse = %#v, %v", command, got, err)
		}
	}
}

func TestParseValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing server", []string{"run", "github"}},
		{"invalid provider", []string{"run", "bitbucket", "--cluster-server", "https://api.example"}},
		{"invalid duration", []string{"run", "github", "--cluster-server", "https://api.example", "--build-timeout", "0s"}},
		{"empty fixture", []string{"run", "github", "--cluster-server", "https://api.example", "--application", ""}},
		{"invalid resume", []string{"run", "github", "--cluster-server", "https://api.example", "--resume", "bad/run"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.args); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCredentialsComeFromEnvironment(t *testing.T) {
	values := map[string]string{
		"GITHUB_TOKEN": "gh-secret", "MY_GITHUB_ORG": "org", "GITLAB_BOT_TOKEN": "gl-secret", "GITLAB_API_URL": "https://gitlab.example/api/v4", "GITLAB_GROUP_ID": "42",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	got, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Credentials.GitHubToken != values["GITHUB_TOKEN"] || got.Credentials.GitLabToken != values["GITLAB_BOT_TOKEN"] {
		t.Fatalf("credentials were not loaded: %#v", got.Credentials)
	}
}

func TestTimeoutsMustBePositive(t *testing.T) {
	req := Defaults()
	req.Command = CommandRun
	req.Provider = ProviderGitHub
	req.ClusterServer = "https://api.example"
	req.Timeouts.Build = -time.Second
	if err := req.Validate(); err == nil || !strings.Contains(err.Error(), "build timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseDoesNotPersistCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-value")
	t.Setenv("MY_GITHUB_ORG", "org")
	got, err := Parse([]string{"run", "github", "--cluster-server", "https://api.example"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.SourceRepository, os.Getenv("GITHUB_TOKEN")) {
		t.Fatal("credential leaked into request configuration")
	}
}

func TestValidateRequiresProviderCredentials(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		request  Request
		want     string
	}{
		{
			name:     "github token",
			provider: ProviderGitHub,
			request:  Request{Credentials: Credentials{GitHubOrg: "org"}},
			want:     "GITHUB_TOKEN",
		},
		{
			name:     "github organization",
			provider: ProviderGitHub,
			request:  Request{Credentials: Credentials{GitHubToken: "token"}},
			want:     "MY_GITHUB_ORG",
		},
		{
			name:     "gitlab token",
			provider: ProviderGitLab,
			request:  Request{Credentials: Credentials{GitLabGroupID: "42"}},
			want:     "GITLAB_BOT_TOKEN",
		},
		{
			name:     "gitlab group",
			provider: ProviderGitLab,
			request:  Request{Credentials: Credentials{GitLabToken: "token"}},
			want:     "GITLAB_GROUP_ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Defaults()
			req.Command = CommandRun
			req.Provider = tt.provider
			req.ClusterServer = "https://api.example"
			req.Credentials = tt.request.Credentials
			if err := req.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %s", err, tt.want)
			}
		})
	}
}
