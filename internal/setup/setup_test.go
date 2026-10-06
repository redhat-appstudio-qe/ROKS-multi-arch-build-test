package setup

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Prepare", func() {
	It("invokes setup script and adds cluster server", func() {
		tmpDir := GinkgoT().TempDir()
		envFile := filepath.Join(tmpDir, "konflux.env")
		if err := os.WriteFile(envFile, []byte("KUBECONFIG=kubeconfig\nGITHUB_TOKEN=test-token\n"), 0600); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "kubeconfig"), []byte("apiVersion: v1\n"), 0600); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		script := filepath.Join(tmpDir, "setup.sh")
		if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf 'KUBECONFIG=%s\\n' /tmp/test-kubeconfig\nprintf 'CLUSTER_SERVER=%s\\n' https://api.example:6443\nprintf 'ENV_FILE=%s\\n' /tmp/test.env\n"), 0700); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		DeferCleanup(os.Setenv, "KONFLUX_TEST_SETUP_SCRIPT", os.Getenv("KONFLUX_TEST_SETUP_SCRIPT"))
		DeferCleanup(os.Setenv, "KUBECONFIG", os.Getenv("KUBECONFIG"))
		Expect(os.Setenv("KONFLUX_TEST_SETUP_SCRIPT", script)).To(Succeed())
		Expect(os.Setenv("KUBECONFIG", "")).To(Succeed())

		args, err := Prepare([]string{"run", "github", "--env-file", envFile})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--cluster-server https://api.example:6443") {
			Expect(joined).To(ContainSubstring("--cluster-server https://api.example:6443"))
		}
		Expect(joined).To(ContainSubstring("--env-file " + envFile))
		Expect(os.Getenv("KUBECONFIG")).To(Equal("/tmp/test-kubeconfig"))
	})

	It("rejects an explicit server mismatch", func() {
		tmpDir := GinkgoT().TempDir()
		script := filepath.Join(tmpDir, "setup.sh")
		if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf 'KUBECONFIG=%s\\n' /tmp/test-kubeconfig\nprintf 'CLUSTER_SERVER=%s\\n' https://api.example:6443\nprintf 'ENV_FILE=%s\\n' /tmp/test.env\n"), 0700); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		DeferCleanup(os.Setenv, "KONFLUX_TEST_SETUP_SCRIPT", os.Getenv("KONFLUX_TEST_SETUP_SCRIPT"))
		Expect(os.Setenv("KONFLUX_TEST_SETUP_SCRIPT", script)).To(Succeed())

		_, err := Prepare([]string{"run", "github", "--cluster-server", "https://other.example:6443"})
		Expect(err).To(MatchError(ContainSubstring("cluster server mismatch")))
	})
})
