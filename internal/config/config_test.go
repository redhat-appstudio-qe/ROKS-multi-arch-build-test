package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseRunCommandUsesCanonicalFixture(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	got, err := Parse([]string{"run", "github", "--cluster-server", "https://api.example:6443"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != CommandRun || got.Provider != ProviderGitHub || got.ClusterServer != "https://api.example:6443" {
		t.Fatalf("unexpected command: %#v", got)
	}
	if got.SourceRepository != DefaultGitHubRepo || got.TenantNamespace != DefaultGitHubTenantNamespace {
		t.Fatalf("unexpected defaults: %#v", got)
	}
}

func TestParseUsesProviderSpecificFixturesAndNamespaces(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	t.Setenv("GITLAB_BOT_TOKEN", "token")
	githubRequest, err := Parse([]string{"run", "github", "--cluster-server", "https://api.example", "--source-repository", DefaultGitHubRepo})
	if err != nil {
		t.Fatal(err)
	}
	gitlabRequest, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example"})
	if err != nil {
		t.Fatal(err)
	}
	if gitlabRequest.SourceRepository != DefaultGitLabRepo || githubRequest.TenantNamespace == gitlabRequest.TenantNamespace {
		t.Fatalf("requests = %#v %#v", githubRequest, gitlabRequest)
	}
}

func TestParseRejectsNonCanonicalFixture(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	_, err := Parse([]string{"run", "github", "--cluster-server", "https://api.example", "--source-repository", "https://github.com/other/repo"})
	if err == nil || !strings.Contains(err.Error(), "fixture must be") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseHonorsExplicitTenantNamespace(t *testing.T) {
	t.Setenv("GITLAB_BOT_TOKEN", "token")
	got, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example", "--tenant-namespace", "custom-tenant"})
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantNamespace != "custom-tenant" {
		t.Fatalf("tenant namespace = %q", got.TenantNamespace)
	}
}

func TestParseLoadsCredentialsFromEnvFile(t *testing.T) {
	envFile := t.TempDir() + "/konflux.env"
	if err := os.WriteFile(envFile, []byte("GITLAB_BOT_TOKEN='file-token'\nGITLAB_API_URL=https://gitlab.example/api/v4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITLAB_BOT_TOKEN", "")
	t.Setenv("GITLAB_API_URL", "")
	got, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example", "--env-file", envFile})
	if err != nil {
		t.Fatal(err)
	}
	if got.Credentials.GitLabToken != "file-token" || got.Credentials.GitLabAPIURL != "https://gitlab.example/api/v4" {
		t.Fatalf("credentials = %#v", got.Credentials)
	}
}

func TestParseRejectsObsoleteCommands(t *testing.T) {
	for _, command := range []string{"legacy-collection", "legacy-cleanup"} {
		if _, err := Parse([]string{command, "run-123", "--cluster-server", "https://api.example"}); err == nil {
			t.Errorf("%s was accepted", command)
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

func TestTimeoutsMustBePositive(t *testing.T) {
	req := Defaults()
	req.Command = CommandRun
	req.Provider = ProviderGitHub
	req.ClusterServer = "https://api.example"
	req.Credentials.GitHubToken = "token"
	req.Timeouts.Build = -time.Second
	if err := req.Validate(); err == nil || !strings.Contains(err.Error(), "build timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiresProviderToken(t *testing.T) {
	for _, provider := range []string{ProviderGitHub, ProviderGitLab} {
		req := Defaults()
		req.Command = CommandRun
		req.Provider = provider
		req.ClusterServer = "https://api.example"
		if err := req.Validate(); err == nil {
			t.Fatalf("provider %s accepted missing credentials", provider)
		}
	}
}
