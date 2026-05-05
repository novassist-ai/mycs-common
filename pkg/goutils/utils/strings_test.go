package utils_test

import (
	"regexp"

	"github.com/novassist/mycs-common/pkg/goutils/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("string utils tests", func() {

	Context("joining list of items to a sentance", func() {

		It("creates output of unquoted items", func() {

			s := utils.JoinListAsSentence(
				"John ate %s for breakfast.",
				[]string{"eggs", "bacon", "baked beans"},
				false,
			)

			Expect(s).To(Equal("John ate eggs, bacon and baked beans for breakfast."))
		})

		It("creates output of quoted items", func() {

			s := utils.JoinListAsSentence(
				"The attendees names were %s.",
				[]string{"Jack", "Jill", "Jane", "Mike"},
				true,
			)

			Expect(s).To(Equal("The attendees names were \"Jack\", \"Jill\", \"Jane\" and \"Mike\"."))
		})
	})

	Context("wrapping a long string with indentation", func() {

		It("splits and indents all lines except the first of a long string", func() {

			s, l := utils.FormatMultilineString(
				"Terraform is a tool for building, changing, and versioning infrastructure safely and efficiently. Terraform can manage existing and popular service providers as well as custom in-house solutions.",
				11, 80, false, true,
			)
			Expect(l).To(BeTrue())

			Expect(s).To(
				Equal(
					`Terraform is a tool for building, changing, and versioning infrastructure safely
           and efficiently. Terraform can manage existing and popular service providers as
           well as custom in-house solutions.`,
				),
			)
		})

		It("splits and indents all lines of a long string", func() {

			s, l := utils.FormatMultilineString(
				"Terraform is a tool for building, changing, and versioning infrastructure safely and efficiently. Terraform can manage existing and popular service providers as well as custom in-house solutions.",
				11, 80, true, true,
			)
			Expect(l).To(BeTrue())

			Expect(s).To(
				Equal(
					`           Terraform is a tool for building, changing, and versioning infrastructure safely
           and efficiently. Terraform can manage existing and popular service providers as
           well as custom in-house solutions.`,
				),
			)
		})

		It("removes whitespace at split", func() {

			s, l := utils.FormatMultilineString(
				"Terraform is a tool for building, changing, and versioning infrastructure            and efficiently. Terraform can manage existing and popular service providers as well as custom in-house solutions.",
				11, 80, false, true,
			)
			Expect(l).To(BeTrue())

			Expect(s).To(
				Equal(
					`Terraform is a tool for building, changing, and versioning infrastructure
           and efficiently. Terraform can manage existing and popular service providers as
           well as custom in-house solutions.`,
				),
			)
		})
	})

	Context("matching lines in a buffer", func() {
		
		It("matches lines against a given search pattern", func() {

			testInput := []byte(`Routing tables

Internet:
Destination        Gateway            Flags        Netif Expire
default            10.20.110.16       UGScg        utun2       
default            192.168.1.1        UGScIg         en0
default            utun7              UGScIg         en0
3.7.35/25          192.168.1.1        UGSc           en0       
3.21.137.128/25    192.168.1.1        UGSc           en0       
3.22.11/24         192.168.1.1        UGSc           en0       
3.23.93/24         192.168.1.1        UGSc           en0       
3.25.41.128/25     192.168.1.1        UGSc           en0       
3.25.42/25         192.168.1.1        UGSc           en0       
3.25.49/24         192.168.1.1        UGSc           en0       
3.208.72/25        192.168.1.1        UGSc           en0       
3.211.241/25       192.168.1.1        UGSc           en0       
3.235.69/25        192.168.1.1        UGSc           en0       
10.20.110.16/32    link#16            UCS          utun2       
13.52.6.128/25     192.168.1.1        UGSc           en0       
13.52.146/25       192.168.1.1        UGSc           en0       
13.107.4.52        192.168.1.1        UGHS           en0       
213.244.140        192.168.1.1        UGSc           en0       
221.122.88.64/27   192.168.1.1        UGSc           en0       
221.122.88.128/25  192.168.1.1        UGSc           en0       
221.122.89.128/25  192.168.1.1        UGSc           en0       
221.123.139.192/27 192.168.1.1        UGSc           en0       
224.0.0/4          link#16            UmCS         utun2       
224.0.0/4          link#6             UmCSI          en0      !
224.0.0.251        link#16            UHmW3I       utun2     93
239.255.255.250    1:0:5e:7f:ff:fa    UHmLWI         en0       
239.255.255.250    link#16            UHmW3I       utun2     32
255.255.255.255/32 link#16            UCS          utun2       
255.255.255.255/32 link#6             UCSI           en0      !`)

			results := utils.ExtractMatches(testInput, map[string]*regexp.Regexp{
				"defaultGateways": regexp.MustCompile(`^default\s+([.0-9a-z]+)\s+\S+\s+(\S+[0-9]+)\s*$`),
				"allGatewayRoutes": regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+(/[0-9]+)?)\s+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)\s+\S+\s+(\S+[0-9]+)\s*$`),
			})

			Expect(results["defaultGateways"]).To(Equal([][]string{
				{"default            10.20.110.16       UGScg        utun2       ", "10.20.110.16", "utun2"},
        {"default            192.168.1.1        UGScIg         en0", "192.168.1.1", "en0"},
        {"default            utun7              UGScIg         en0", "utun7", "en0"},
			}))
			Expect(results["allGatewayRoutes"]).To(Equal([][]string{
				{"3.21.137.128/25    192.168.1.1        UGSc           en0       ", "3.21.137.128/25", "/25", "192.168.1.1", "en0"},
        {"3.25.41.128/25     192.168.1.1        UGSc           en0       ", "3.25.41.128/25", "/25", "192.168.1.1", "en0"},
        {"13.52.6.128/25     192.168.1.1        UGSc           en0       ", "13.52.6.128/25", "/25", "192.168.1.1", "en0"},
        {"13.107.4.52        192.168.1.1        UGHS           en0       ", "13.107.4.52", "", "192.168.1.1", "en0"},
        {"221.122.88.64/27   192.168.1.1        UGSc           en0       ", "221.122.88.64/27", "/27", "192.168.1.1", "en0"},
        {"221.122.88.128/25  192.168.1.1        UGSc           en0       ", "221.122.88.128/25", "/25", "192.168.1.1", "en0"},
        {"221.122.89.128/25  192.168.1.1        UGSc           en0       ", "221.122.89.128/25", "/25", "192.168.1.1", "en0"},
        {"221.123.139.192/27 192.168.1.1        UGSc           en0       ", "221.123.139.192/27", "/27", "192.168.1.1", "en0"},
			}))
		})
	})
})
