package providers

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestProviders runs the providers package's Ginkgo test suite.
func TestProviders(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Providers Suite")
}
