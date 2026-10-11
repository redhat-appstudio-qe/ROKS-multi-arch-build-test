package evidence

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestEvidence runs the Ginkgo suite for the evidence package.
func TestEvidence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Evidence Suite")
}
