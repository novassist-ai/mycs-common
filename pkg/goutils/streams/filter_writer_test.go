package streams_test

import (
	"io"
	"strings"

	"github.com/novassist/mycs-common/pkg/goutils/streams"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Output filter unit tests", func() {

	var (
		err error
	)

	Context("simple include or exclude all function", func() {

		It("passes through all data written", func() {

			var (
				outBuffer strings.Builder
				filter    streams.Filter
			)

			filter.SetPassThru()
			filterWriter := streams.NewFilterWriter(&filter, &outBuffer)
			err = writeTestData(testData, filterWriter)
			Expect(err).NotTo(HaveOccurred())
			filterWriter.Close()

			Expect(outBuffer.String()).To(Equal(testData))
		})

		It("does not pass through any of the data written", func() {

			var (
				outBuffer strings.Builder
				filter    streams.Filter
			)

			filter.SetBlackHole()
			filterWriter := streams.NewFilterWriter(&filter, &outBuffer)
			err = writeTestData(testData, filterWriter)
			Expect(err).NotTo(HaveOccurred())
			filterWriter.Close()

			Expect(outBuffer.String()).To(Equal(""))
		})
	})

	Context("excludes or includes data occuring after matching patterns", func() {

		It("excludes all data after matching a particular pattern", func() {

			var (
				outBuffer strings.Builder
				filter    streams.Filter
			)

			filter.AddExcludeAfterPattern("^individuals.$")
			filterWriter := streams.NewFilterWriter(&filter, &outBuffer)
			err = writeTestData(testData, filterWriter)
			Expect(err).NotTo(HaveOccurred())
			filterWriter.Close()

			Expect(outBuffer.String()).To(Equal(testDataResult1))
		})

		It("includes all data after matching a particular pattern", func() {

			var (
				outBuffer strings.Builder
				filter    streams.Filter
			)

			filter.AddIncludeAfterPattern("^individuals.$")
			filterWriter := streams.NewFilterWriter(&filter, &outBuffer)
			err = writeTestData(testData, filterWriter)
			Expect(err).NotTo(HaveOccurred())
			filterWriter.Close()

			Expect(outBuffer.String()).To(Equal(testDataResult2))
		})

		It("excludes data after matching a particular pattern and includes data after another pattern", func() {

			var (
				outBuffer strings.Builder
				filter    streams.Filter
			)

			filter.AddExcludeAfterPattern("personal information")
			filter.AddIncludeAfterPattern("attention")
			filterWriter := streams.NewFilterWriter(&filter, &outBuffer)
			err = writeTestData(testData, filterWriter)
			Expect(err).NotTo(HaveOccurred())

			// it is important that close is called
			// before retrieving outBuffer to ensure
			// all data has been flushed
			filterWriter.Close()

			Expect(outBuffer.String()).To(Equal(testDataResult3))
		})

		It("includes data after matching a particular pattern and excludes data after another pattern", func() {

			var (
				outBuffer strings.Builder
				filter    streams.Filter
			)

			filter.AddIncludeAfterPattern("the idea most people")
			filter.AddExcludeAfterPattern("^individuals.$")
			filterWriter := streams.NewFilterWriter(&filter, &outBuffer)
			err = writeTestData(testData, filterWriter)
			Expect(err).NotTo(HaveOccurred())
			filterWriter.Close()

			Expect(outBuffer.String()).To(Equal(testDataResult4))
		})
	})

	Context("excludes or includes data occuring after matching patterns", func() {

		It("includes or excludes lines matching particular patterns", func() {

			var (
				outBuffer strings.Builder
				filter    streams.Filter
			)

			filter.AddIncludeAfterPattern("the idea most people")
			filter.AddExcludeAfterPattern("^individuals.$")
			filter.AddIncludePattern(" of ")
			filter.AddExcludePattern(" rights ")
			filterWriter := streams.NewFilterWriter(&filter, &outBuffer)
			err = writeTestData(testData, filterWriter)
			Expect(err).NotTo(HaveOccurred())
			filterWriter.Close()

			Expect(outBuffer.String()).To(Equal(testDataResult5))
		})
	})
})

func writeTestData(data string, output io.Writer) error {

	var (
		err error

		i, ii, l int
	)

	// write data in blocks of 20 chars
	l = len(data)
	for i = 0; i < l; {
		ii = i + 20
		if ii > l {
			ii = l
		}
		_, err = output.Write([]byte(data[i:ii]))
		Expect(err).NotTo(HaveOccurred())
		i = ii
	}

	return err
}

const testData = `
Privacy is an individual’s right to freedom from intrusion and prying
eyes.

It is guaranteed under the constitution in many developed countries,
which makes it a fundamental human right and one of the core
principles of human dignity, the idea most people will agree about.

Privacy is all about the rights of individuals with respect to their
personal information.

Any risk assessment conducted for the purpose of enhancing the
privacy of individuals’ personal data is performed from the
perspective of protecting the rights and freedoms of those
individuals.

Security is the state of personal freedom or being free from potential 
threats, whereas privacy refers to the state of being free from 
unwanted attention.

However, privacy cannot exist without security first established.`

const testDataResult1 = `
Privacy is an individual’s right to freedom from intrusion and prying
eyes.

It is guaranteed under the constitution in many developed countries,
which makes it a fundamental human right and one of the core
principles of human dignity, the idea most people will agree about.

Privacy is all about the rights of individuals with respect to their
personal information.

Any risk assessment conducted for the purpose of enhancing the
privacy of individuals’ personal data is performed from the
perspective of protecting the rights and freedoms of those
individuals.
`

const testDataResult2 = `
Security is the state of personal freedom or being free from potential 
threats, whereas privacy refers to the state of being free from 
unwanted attention.

However, privacy cannot exist without security first established.`

const testDataResult3 = `
Privacy is an individual’s right to freedom from intrusion and prying
eyes.

It is guaranteed under the constitution in many developed countries,
which makes it a fundamental human right and one of the core
principles of human dignity, the idea most people will agree about.

Privacy is all about the rights of individuals with respect to their
personal information.

However, privacy cannot exist without security first established.`

const testDataResult4 = `
Privacy is all about the rights of individuals with respect to their
personal information.

Any risk assessment conducted for the purpose of enhancing the
privacy of individuals’ personal data is performed from the
perspective of protecting the rights and freedoms of those
individuals.
`

const testDataResult5 = `Any risk assessment conducted for the purpose of enhancing the
privacy of individuals’ personal data is performed from the
`
