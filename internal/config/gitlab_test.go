package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GitLab fixture", func() {
	It("is canonical", func() {
		Expect(DefaultGitLabRepo).To(Equal("https://gitlab.com/konflux-qe/dr_test_mathwizz_gl"))
	})
})
