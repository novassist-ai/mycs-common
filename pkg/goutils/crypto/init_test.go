package crypto_test

import (
	"path"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/onsi/gomega/gexec"
)

var (
	workingDirectory string
)

func TestCrypto(t *testing.T) {
	logger.Initialize()

	_, filename, _, _ := runtime.Caller(0)
	workingDirectory = path.Dir(filename)

	RegisterFailHandler(Fail)
	RunSpecs(t, "crypto")
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
