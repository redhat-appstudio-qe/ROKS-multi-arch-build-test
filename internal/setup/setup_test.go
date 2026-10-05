package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareInvokesSetupScriptAndAddsClusterServer(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, "konflux.env")
	if err := os.WriteFile(envFile, []byte("KUBECONFIG=kubeconfig\nGITHUB_TOKEN=test-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "kubeconfig"), []byte("apiVersion: v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(tmpDir, "setup.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf 'KUBECONFIG=%s\\n' /tmp/test-kubeconfig\nprintf 'CLUSTER_SERVER=%s\\n' https://api.example:6443\nprintf 'ENV_FILE=%s\\n' /tmp/test.env\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KONFLUX_TEST_SETUP_SCRIPT", script)
	t.Setenv("KUBECONFIG", "")

	args, err := Prepare([]string{"run", "github", "--env-file", envFile})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--cluster-server https://api.example:6443") {
		t.Fatalf("args = %q", joined)
	}
	if !strings.Contains(joined, "--env-file "+envFile) {
		t.Fatalf("args = %q", joined)
	}
	if got := os.Getenv("KUBECONFIG"); got != "/tmp/test-kubeconfig" {
		t.Fatalf("KUBECONFIG = %q", got)
	}
}

func TestPrepareRejectsExplicitServerMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "setup.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf 'KUBECONFIG=%s\\n' /tmp/test-kubeconfig\nprintf 'CLUSTER_SERVER=%s\\n' https://api.example:6443\nprintf 'ENV_FILE=%s\\n' /tmp/test.env\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KONFLUX_TEST_SETUP_SCRIPT", script)

	_, err := Prepare([]string{"run", "github", "--cluster-server", "https://other.example:6443"})
	if err == nil || !strings.Contains(err.Error(), "cluster server mismatch") {
		t.Fatalf("error = %v", err)
	}
}
