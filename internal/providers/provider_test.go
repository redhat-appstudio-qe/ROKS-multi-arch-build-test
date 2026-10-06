package providers

import (
	"context"
	"errors"
	"time"

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

	It("retries a 5xx operation three times with the configured delay", func() {
		policy := RetryPolicy{
			MaxRetries: 3,
			Delay:      3 * time.Minute,
			Sleep: func(_ context.Context, delay time.Duration) error {
				Expect(delay).To(Equal(3 * time.Minute))
				return nil
			},
		}
		attempts := 0
		err := RetryOn5xx(context.Background(), policy, func() error {
			attempts++
			return errors.New("server error")
		}, func(error) bool { return true })

		Expect(err).To(MatchError("server error"))
		Expect(attempts).To(Equal(4))
	})

	It("uses three retries and a three-minute delay by default", func() {
		policy := DefaultRetryPolicy()
		Expect(policy.MaxRetries).To(Equal(3))
		Expect(policy.Delay).To(Equal(3 * time.Minute))
	})

	It("does not retry non-5xx errors", func() {
		attempts := 0
		err := RetryOn5xx(context.Background(), DefaultRetryPolicy(), func() error {
			attempts++
			return errors.New("client error")
		}, func(error) bool { return false })

		Expect(err).To(MatchError("client error"))
		Expect(attempts).To(Equal(1))
	})

})
