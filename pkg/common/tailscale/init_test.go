package tailscale_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/onsi/gomega/gexec"
)

var (
	tmpDir string
)

func TestTailscale(t *testing.T) {
	logger.Initialize()

	tmpDir = filepath.Join(os.TempDir(), "tailscale-tmp")
	
	RegisterFailHandler(Fail)
	RunSpecs(t, "tailscale")
}

var _ = AfterSuite(func() {
	os.Remove(tmpDir)
	gexec.CleanupBuildArtifacts()
})
