package utils_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

func TestUtils(t *testing.T) {
	logger.Initialize()

	RegisterFailHandler(Fail)
	RunSpecs(t, "utils")
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
