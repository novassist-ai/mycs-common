package streams_test

import (
	"bytes"

	"github.com/novassist/mycs-common/pkg/goutils/streams"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("bytes utils tests", func() {

	Context("writes to buffer", func() {

		var (
			err    error
			output bytes.Buffer
		)

		It("writes at random position", func() {

			writeAtBuffer := streams.NewWriteAtBuffer(&output)

			_, err = writeAtBuffer.WriteAt([]byte("abcd"), 10)
			Expect(err).ToNot(HaveOccurred())
			_, err = writeAtBuffer.WriteAt([]byte("56789"), 5)
			Expect(err).ToNot(HaveOccurred())
			Expect(output.Len()).To(Equal(0))

			_, err = writeAtBuffer.WriteAt([]byte("01234"), 0)
			Expect(err).ToNot(HaveOccurred())
			_, err = writeAtBuffer.WriteAt([]byte("fghij"), 15)
			Expect(err).ToNot(HaveOccurred())
			Expect(output.String()).To(Equal("0123456789abcd"))

			_, err = writeAtBuffer.WriteAt([]byte("mno"), 22)
			Expect(err).ToNot(HaveOccurred())
			_, err = writeAtBuffer.WriteAt([]byte("e"), 14)
			Expect(err).ToNot(HaveOccurred())
			Expect(output.String()).To(Equal("0123456789abcdefghij"))

			_, err = writeAtBuffer.WriteAt([]byte("qrstu"), 25)
			Expect(err).ToNot(HaveOccurred())
			_, err = writeAtBuffer.WriteAt([]byte("xyz"), 32)
			Expect(err).ToNot(HaveOccurred())
			_, err = writeAtBuffer.WriteAt([]byte("kl"), 20)
			Expect(err).ToNot(HaveOccurred())
			Expect(output.String()).To(Equal("0123456789abcdefghijklmnoqrstu"))

			// buffer has unwritten data so close should fail
			err = writeAtBuffer.Close()
			Expect(err).To(HaveOccurred())

			_, err = writeAtBuffer.WriteAt([]byte("vw"), 30)
			Expect(err).ToNot(HaveOccurred())
			Expect(output.String()).To(Equal("0123456789abcdefghijklmnoqrstuvwxyz"))

			err = writeAtBuffer.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
