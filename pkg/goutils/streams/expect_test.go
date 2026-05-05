package streams_test

import (
	"bufio"
	"io"
	"strings"

	"github.com/novassist/mycs-common/pkg/goutils/streams"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Expect Stream Interceptor", func() {

	var (
		err error

		outputBuffer strings.Builder
	)

	// send data to writer in chunks of given size
	writeData := func(w io.Writer, d []byte, s int) {

		var (
			j, k, l int
		)

		l = len(d)
		for i := 0; i < l; {
			j = i + s
			if j > l {
				j = l
			}
			k, err = w.Write(d[i:j])
			Expect(err).NotTo(HaveOccurred())
			i = i + k
		}
	}

	// send EOT to simulate session termination
	sendEOT := func(stdinWriter io.WriteCloser, stdinOfSender io.Reader) {

		eot := make([]byte, 1)
		writeData(stdinWriter, []byte{4}, 32)

		// eot should have been transmitted to receiver
		_, err = stdinOfSender.Read(eot)
		Expect(err).NotTo(HaveOccurred())
		Expect(eot).To(Equal([]byte{4}))
	}

	BeforeEach(func() {
		outputBuffer.Reset()
	})

	Context("stream exit tests", func() {

		It("exits when user enters 'ctrl-d' via client input", func() {

			// pipe to send data from client
			stdin, stdinWriter := io.Pipe()
			es, stdinOfSender, stdoutOfSender := streams.NewExpectStream(
				stdin, &outputBuffer,
				func() {
					stdinWriter.Close()
				},
			)

			es.SetBufferSize(32)
			es.StartAsShell()

			writeData(stdoutOfSender, []byte(testRecieverWelcome), 32)
			sendEOT(stdinWriter, stdinOfSender)

			// ensure all data is flushed
			es.Close()

			Expect(outputBuffer.String()).To(Equal(
				testRecieverWelcome,
			))
		})

		It("exits when user enters 'exit' via client input", func() {

			// pipe to send data from client
			stdin, stdinWriter := io.Pipe()
			es, stdinOfSender, stdoutOfSender := streams.NewExpectStream(
				stdin, &outputBuffer,
				func() {
					stdinWriter.Close()
				},
			)

			es.SetBufferSize(32)
			es.StartAsShell()

			// reader from which data sent to receiver can be retrieved
			recieverData := bufio.NewScanner(stdinOfSender)

			writeData(stdoutOfSender, []byte(testRecieverWelcome), 32)
			writeData(stdinWriter, []byte("exit\n"), 32)
			recieverData.Scan()
			command := recieverData.Text()
			Expect(command).To(Equal("exit"))

			// ensure all data is flushed
			es.Close()

			Expect(outputBuffer.String()).To(Equal(
				testRecieverWelcome,
			))
		})
	})

	Context("send commands", func() {

		It("receives commands from expect stream", func() {

			// pipe to send data from client
			stdin, stdinWriter := io.Pipe()

			es, stdinOfSender, stdoutOfSender := streams.NewExpectStream(
				stdin, &outputBuffer,
				func() {
					stdinWriter.Close()
				},
			)
			defer func() {
				es.Close()
			}()

			es.SetBufferSize(32)
			es.AddExpectOutTrigger(
				&streams.Expect{
					StartPattern: `^Welcome to Ubuntu`,
					EndPattern:   `^bastion-admin@cbs-test:\~\$`,
					Command:      "sudo su -\n",
				},
				true,
			)
			es.AddExpectOutTrigger(
				&streams.Expect{
					StartPattern: `password for bastion-admin:`,
					Command:      "P@ssw0rd!\n",
				},
				true,
			)
			es.AddExpectOutTrigger(
				&streams.Expect{
					StartPattern: `root@cbs-test:\~\#`,
					Command:      "ls -al /usr\n",
				},
				true,
			)
			es.StartAsShell()

			// reader from which data sent to receiver can be retrieved
			recieverData := bufio.NewScanner(stdinOfSender)

			writeData(stdoutOfSender, []byte(testRecieverWelcome), 32)
			recieverData.Scan()
			command := recieverData.Text()
			Expect(command).To(Equal("sudo su -"))

			writeData(stdoutOfSender, []byte(testRecieverSudoPassword), 32)
			recieverData.Scan()
			command = recieverData.Text()
			Expect(command).To(Equal("P@ssw0rd!"))

			writeData(stdoutOfSender, []byte(testRecieverSudoPrompt), 32)
			recieverData.Scan()
			command = recieverData.Text()
			Expect(command).To(Equal("ls -al /usr"))
			writeData(stdoutOfSender, []byte(testRecieverListOutput), 32)

			sendEOT(stdinWriter, stdinOfSender)

			// ensure all data is flushed
			es.Close()

			Expect(outputBuffer.String()).To(Equal(
				testRecieverWelcome + testRecieverSudoPassword + testRecieverSudoPrompt + testRecieverListOutput,
			))
		})

		It("receives commands from expect stream and user input", func() {

			// pipe to send data from client
			stdin, stdinWriter := io.Pipe()

			es, stdinOfSender, stdoutOfSender := streams.NewExpectStream(
				stdin, &outputBuffer,
				func() {
					stdinWriter.Close()
				},
			)
			defer func() {
				es.Close()
			}()

			es.SetBufferSize(32)
			es.AddExpectOutTrigger(
				&streams.Expect{
					StartPattern: `^Welcome to Ubuntu`,
					EndPattern:   `^bastion-admin@cbs-test:\~\$`,
					Command:      "sudo su -\n",
				},
				true,
			)
			es.AddExpectOutTrigger(
				&streams.Expect{
					StartPattern: `password for bastion-admin:`,
					Command:      "P@ssw0rd!\n",
				},
				true,
			)
			es.StartAsShell()

			// reader from which data sent to receiver can be retrieved
			recieverData := bufio.NewScanner(stdinOfSender)

			writeData(stdoutOfSender, []byte(testRecieverWelcome), 32)
			recieverData.Scan()
			command := recieverData.Text()
			Expect(command).To(Equal("sudo su -"))

			writeData(stdoutOfSender, []byte(testRecieverSudoPassword), 32)
			recieverData.Scan()
			command = recieverData.Text()
			Expect(command).To(Equal("P@ssw0rd!"))

			writeData(stdoutOfSender, []byte(testRecieverSudoPrompt), 32)
			writeData(stdinWriter, []byte("ls -al /usr\n"), 32)
			recieverData.Scan()
			command = recieverData.Text()
			Expect(command).To(Equal("ls -al /usr"))
			writeData(stdoutOfSender, []byte(testRecieverListOutput), 32)

			sendEOT(stdinWriter, stdinOfSender)

			// ensure all data is flushed
			es.Close()

			Expect(outputBuffer.String()).To(Equal(
				testRecieverWelcome + testRecieverSudoPassword + testRecieverSudoPrompt + testRecieverListOutput,
			))
		})
	})
})

const testRecieverWelcome = `Welcome to Ubuntu 18.04.3 LTS (GNU/Linux 5.0.0-1028-gcp x86_64)

* Documentation:  https://help.ubuntu.com
* Management:     https://landscape.canonical.com
* Support:        https://ubuntu.com/advantage

 System information as of Fri Jan 10 19:09:11 UTC 2020

 System load:                    0.84
 Usage of /:                     7.5% of 48.29GB
 Memory usage:                   9%
 Swap usage:                     0%
 Processes:                      344
 Users logged in:                0
 IP address for ens0:            192.168.0.1

* Overheard at KubeCon: "microk8s.status just blew my mind".

		https://microk8s.io/docs/commands#microk8s.status

* Canonical Livepatch is available for installation.
	- Reduce system reboots and improve kernel security. Activate at:
		https://ubuntu.com/livepatch

47 packages can be updated.
1 update is a security update.


Last login: Fri Jan 10 09:23:26 2020 from 94.202.78.17
bastion-admin@cbs-test:~$ `

const testRecieverSudoPassword = `[sudo] password for bastion-admin: `
const testRecieverSudoPrompt = `root@cbs-test:~# `
const testRecieverListOutput = `total 76
drwxr-xr-x  11 root root  4096 Nov 13 15:53 .
drwxr-xr-x  24 root root  4096 Jan 10 22:08 ..
drwxr-xr-x   2 root root 36864 Dec 18 09:19 bin
drwxr-xr-x   2 root root  4096 Apr 24  2018 games
drwxr-xr-x  43 root root  4096 Nov 13 15:51 include
drwxr-xr-x  79 root root  4096 Dec 18 09:17 lib
drwxr-xr-x   3 root root  4096 Nov 13 15:53 libexec
drwxr-xr-x  10 root root  4096 Nov 13 04:33 local
drwxr-xr-x   2 root root  4096 Dec 18 09:18 sbin
drwxr-xr-x 137 root root  4096 Dec 18 09:17 share
drwxr-xr-x   9 root root  4096 Jan  9 06:03 src
`
