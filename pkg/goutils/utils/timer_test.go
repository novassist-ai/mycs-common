package utils_test

import (
	"context"
	"fmt"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("timer utils tests", func() {

	var (
		err error

		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("triggers callback at specified intervals", func() {

		var (
			counter int

			currInterval,
			lastInvocation time.Duration
		)
		counter = 0

		callback := func() (time.Duration, error) {
			defer GinkgoRecover()

			timeNow := time.Duration(time.Now().UnixNano() / int64(time.Millisecond))
			if counter > 0 {
				Expect(timeNow - lastInvocation).To(BeNumerically("~", currInterval, 10))
			}
			lastInvocation = timeNow

			counter++
			switch counter {
				case 1: {
					// initial interval
					currInterval = 500
				}
				case 3: {
					// 0..500..500 => 1s
					currInterval = 1000
				}
				case 4: {
					// 1s..1000 => 2s
					currInterval = 250
				}
				case 8: {
					// 2s..250..250..250..250 => 3s
					currInterval = 1500

					//3s..1500..1500 => 5s at stop
				}
				default: {
				// no change in interval
				return 0, nil					
				}
			}
			return currInterval, nil 
		}

		timer := utils.NewExecTimer(ctx, callback, false)
		err = timer.Start(0)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(time.Millisecond * 5050)
		err = timer.Stop()
		Expect(err).ToNot(HaveOccurred())
		Expect(counter).To(Equal(9))
	})

	It("starts the timer after an initial delay and exists with error after 2s", func() {

		var (
			counter int

			currInterval,
			lastInvocation time.Duration
		)
		counter = 0
		currInterval = 500
		lastInvocation = time.Duration(time.Now().UnixNano() / int64(time.Millisecond))
		
		callback := func() (time.Duration, error) {
			defer GinkgoRecover()

			timeNow := time.Duration(time.Now().UnixNano() / int64(time.Millisecond))
			Expect(timeNow - lastInvocation).To(BeNumerically("~", currInterval, 10))
			lastInvocation = timeNow

			counter++
			switch counter {
				case 3: {
					// ..500..500 => 1s
					currInterval = 1000
				}
				case 5: {
					// ..1000..1000 => 3s
					return 0, fmt.Errorf("callback raised error")
				}
				// no change in interval
				default: {
					return 0, nil					
				}
			}
			return currInterval, nil 
		}

		timer := utils.NewExecTimer(ctx, callback, true)
		err = timer.Start(currInterval)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(time.Millisecond * 5000)
		err = timer.Stop()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("callback raised error"))
		Expect(counter).To(Equal(5))
	})
})
