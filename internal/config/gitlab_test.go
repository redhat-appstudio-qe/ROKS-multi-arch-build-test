package config

import "testing"

func TestGitLabFixtureIsCanonical(t *testing.T) {
	if DefaultGitLabRepo != "https://gitlab.com/konflux-qe/dr_test_mathwizz_gl" {
		t.Fatalf("gitlab fixture = %q", DefaultGitLabRepo)
	}
}
