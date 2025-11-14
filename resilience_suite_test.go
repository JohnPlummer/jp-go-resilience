package resilience_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestResilience(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resilience Suite")
}
