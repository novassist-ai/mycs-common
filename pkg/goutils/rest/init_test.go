package rest_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/onsi/gomega/gexec"
)

func TestRest(t *testing.T) {
	logger.Initialize()

	RegisterFailHandler(Fail)
	RunSpecs(t, "rest")
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
