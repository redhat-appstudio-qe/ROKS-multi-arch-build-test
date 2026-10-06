package providers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("repository helpers", func() {
	It("parses a repository", func() {
		got, err := ParseRepository("https://gitlab.com/konflux-qe/dr_test_mathwizz_gl.git")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Owner).To(Equal("konflux-qe"))
		Expect(got.Name).To(Equal("dr_test_mathwizz_gl"))
	})

	It("rejects a stale expected SHA", func() {
		Expect(ValidateExpectedSHA("old", "new")).To(HaveOccurred())
	})

	It("parses repository URL without a suffix", func() {
		got, err := ParseRepository("https://gitlab.com/konflux-qe/dr_test_mathwizz_gl")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.URL).To(Equal("https://gitlab.com/konflux-qe/dr_test_mathwizz_gl"))
	})
})
