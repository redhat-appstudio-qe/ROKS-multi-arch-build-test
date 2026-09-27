package config

import "testing"

func TestGitLabUsesLiveMathWizzFixtureByDefault(t *testing.T) {
	t.Setenv("GITLAB_BOT_TOKEN", "token")
	t.Setenv("GITLAB_GROUP_ID", "42")
	got, err := Parse([]string{"run", "gitlab", "--cluster-server", "https://api.example"})
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceRepository != DefaultGitLabRepo {
		t.Fatalf("source repository = %q, want %q", got.SourceRepository, DefaultGitLabRepo)
	}
	if got.TenantNamespace != DefaultGitLabTenantNamespace {
		t.Fatalf("tenant namespace = %q, want %q", got.TenantNamespace, DefaultGitLabTenantNamespace)
	}
}
