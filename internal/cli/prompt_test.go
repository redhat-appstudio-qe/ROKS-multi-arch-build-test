package cli

import (
	"bytes"
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfirmationPrompt", func() {
	It("accepts only explicit approval", func() {
		for _, answer := range []string{"y\n", "yes\n", "Y\n", "YES\n"} {
			prompt := NewConfirmationPrompt(strings.NewReader(answer), io.Discard, true)
			approved, err := prompt.Confirm(context.Background(), "delete tenant")
			Expect(err).NotTo(HaveOccurred())
			Expect(approved).To(BeTrue(), "answer %q", answer)
		}
	})

	It("refuses unsafe input", func() {
		for _, answer := range []string{"n\n", "no\n", "\n", "maybe\n", ""} {
			prompt := NewConfirmationPrompt(strings.NewReader(answer), io.Discard, true)
			approved, err := prompt.Confirm(context.Background(), "delete tenant")
			Expect(err).NotTo(HaveOccurred())
			Expect(approved).To(BeFalse(), "answer %q", answer)
		}
		prompt := NewConfirmationPrompt(strings.NewReader("y\n"), io.Discard, false)
		approved, err := prompt.Confirm(context.Background(), "delete tenant")
		Expect(err).NotTo(HaveOccurred())
		Expect(approved).To(BeFalse())
	})

	It("does not print when no decision is possible", func() {
		var output bytes.Buffer
		prompt := NewConfirmationPrompt(strings.NewReader(""), &output, true)
		approved, err := prompt.Confirm(context.Background(), "delete tenant")
		Expect(err).NotTo(HaveOccurred())
		Expect(approved).To(BeFalse())
		Expect(output.Len()).To(BeNumerically(">", 0))
		prompt = NewConfirmationPrompt(strings.NewReader("y\n"), &output, false)
		output.Reset()
		_, _ = prompt.Confirm(context.Background(), "delete tenant")
		Expect(output.Len()).To(Equal(0))
	})
})
