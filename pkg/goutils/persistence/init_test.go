package persistence_test

import (
	"testing"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestData(t *testing.T) {
	logger.Initialize()

	RegisterFailHandler(Fail)
	RunSpecs(t, "persistence")
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
