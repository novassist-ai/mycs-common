package tailscale_test

import (
	"strings"

	"github.com/novassist/mycs-common/pkg/common/tailscale"
	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/run"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tailscale Daemon", func() {

	It("start the tailscale daemon and validate it starts up without errors", func() {
		isAdmin, err := run.IsAdmin()
		Expect(err).NotTo(HaveOccurred())
		if !isAdmin {
			Skip("requires root: sudo -E go test -v ./pkg/common/tailscale")
		}

		var (
			outputBuffer strings.Builder
		)

		tsd := tailscale.NewTailscaleDaemon(tmpDir, &outputBuffer)
		err = tsd.Start()
		Expect(err).ToNot(HaveOccurred())
		tsd.Stop()

		output := outputBuffer.String()
		logger.DebugMessage("Tailscale Daemon log: \n%s\n", output)
		Expect(output).To(ContainSubstring("logtail started"))
		Expect(output).To(ContainSubstring("Program starting:"))
		Expect(output).To(ContainSubstring("LogID:"))
		Expect(output).To(ContainSubstring("logpolicy:"))
		Expect(output).To(ContainSubstring("flushing log."))
		Expect(output).To(ContainSubstring("logger closing down"))
	})
})
