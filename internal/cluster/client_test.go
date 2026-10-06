package cluster

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

var _ = Describe("NewClientSetFromConfig", func() {
	It("bounds default HTTP requests without mutating the input", func() {
		config := &rest.Config{Host: "https://api.example"}
		clients, err := NewClientSetFromConfig(config)
		Expect(err).NotTo(HaveOccurred())
		Expect(clients.Config.Timeout).To(Equal(20 * time.Second))
		Expect(config.Timeout).To(BeZero())
	})
})
