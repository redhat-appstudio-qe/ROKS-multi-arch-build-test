package evidence

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RedactedJSON", func() {
	It("redacts nested sensitive fields", func() {
		value := map[string]any{
			"name":   "run-1",
			"token":  "top-secret",
			"nested": map[string]any{"password": "also-secret", "value": "kept"},
			"items":  []any{map[string]any{"authorizationHeader": "bearer"}},
		}
		data, err := RedactedJSON(value)
		Expect(err).NotTo(HaveOccurred())
		text := string(data)
		Expect(text).NotTo(ContainSubstring("top-secret"))
		Expect(text).NotTo(ContainSubstring("also-secret"))
		Expect(text).NotTo(ContainSubstring("bearer"))
		Expect(strings.Contains(text, "[REDACTED]")).To(BeTrue())
		Expect(strings.Contains(text, "kept")).To(BeTrue())
	})
})
