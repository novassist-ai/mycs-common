package network_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/novassist/mycs-common/pkg/common/network"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Port Mapper", func() {

	var (
		err error

		testServer *http.Server
		localPort  uint16
	)

	BeforeEach(func() {
		listener, err := net.Listen("tcp", ":0")
		Expect(err).ToNot(HaveOccurred())

		localPort = uint16(listener.Addr().(*net.TCPAddr).Port)

		testServer = &http.Server{
			Handler: http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprintln(w, "OK")
				},
			),
		}
		go func() {
			_ = testServer.Serve(listener)
		}()
	})

	AfterEach(func() {
		_ = testServer.Shutdown(context.Background())
	})

	It("Creates a forwarding rule with expiration", func() {

		pm := network.NewPortMapper(context.Background(), 5000)
		err = pm.Connect(30 * time.Second)
		if err != nil {
			Skip("UPnP / gateway not available on this network or host: " + err.Error())
		}
		defer pm.Close()

		err = pm.AddPortMappingToSelf("test1", network.ProtocolTCP, 48000, localPort, 10 * time.Second)
		Expect(err).ToNot(HaveOccurred())

		client := http.Client{
			Timeout: time.Second,
		}
		resp, err := client.Get("http://" + pm.ExternalIP() + ":48000");
		Expect(err).ToNot(HaveOccurred())
	
		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(body)).To(Equal("OK"))

		time.Sleep(10 * time.Second)

		_, err = client.Get("http://" + pm.ExternalIP() + ":48000");
		Expect(err).To(HaveOccurred())
	})
})
