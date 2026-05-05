package mycsnode_test

import (
	"testing"

	"github.com/novassist/mycs-common/pkg/goutils/logger"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMyCSNode(t *testing.T) {
	logger.Initialize()

	RegisterFailHandler(Fail)
	RunSpecs(t, "mycsnode")
}

var _ = AfterSuite(func() {
})
