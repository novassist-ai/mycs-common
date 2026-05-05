package auth_test

import (
	"testing"

	"github.com/novassist/mycs-common/pkg/goutils/logger"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAuth(t *testing.T) {
	logger.Initialize()

	RegisterFailHandler(Fail)
	RunSpecs(t, "auth")
}

var _ = AfterSuite(func() {
})
