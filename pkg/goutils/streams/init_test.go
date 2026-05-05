package streams_test

import (
	"testing"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRun(t *testing.T) {
	logger.Initialize()

	RegisterFailHandler(Fail)
	RunSpecs(t, "streams")
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
