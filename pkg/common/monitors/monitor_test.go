package monitors_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/novassist/mycs-common/pkg/common/events"
	"github.com/novassist/mycs-common/pkg/common/monitors"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/novassist/mycs-common/pkg/goutils/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Monitors", func() {

	var (
		err error
	)

	It("collects from an incrementing and decrementing monitor counter", func() {

		s := &testSender{}
		msvc := monitors.NewMonitorService(s, 5, 1000)

		monitor := msvc.NewMonitor("testMonitor")
		Expect(monitor).NotTo(BeNil())
		counter := monitors.NewCounter("testCounter", false, false)
		Expect(counter).NotTo(BeNil())
		monitor.AddCounter(counter)

		err = msvc.Start()
		Expect(err).NotTo(HaveOccurred())

		wg := sync.WaitGroup{}
		wg.Add(3)

		cumalativeValue := 0
		incAt := []time.Duration{100, 200, 500}
		for i := 0; i < 3; i++ {
			incInt := incAt[i]

			go func() {
				t := time.Duration(0)
				for t < 15500 {
					timer := time.NewTicker(incInt * time.Millisecond)
					<-timer.C
					counter.Inc()
					cumalativeValue++
					t += incInt
				}
				wg.Done()
			}()
		}
		wg.Wait()

		msvc.Stop()
		Expect(s.numEvents).To(Equal(16))
		// Each event carries a snapshot of the running counter, not a delta; summing
		// snapshot values across posts is not comparable to total Inc() count.
		Expect(counter.Get()).To(Equal(int64(cumalativeValue)))
	})

	It("collects from an incrementing and decrementing monitor counter", func() {

		s := &testSender{}
		msvc := monitors.NewMonitorService(s, 5, 1000)

		monitor := msvc.NewMonitor("testMonitor")
		Expect(monitor).NotTo(BeNil())
		counter := monitors.NewCounter("testCounter", true, false)
		Expect(counter).NotTo(BeNil())
		monitor.AddCounter(counter)

		err = msvc.Start()
		Expect(err).NotTo(HaveOccurred())

		wg := sync.WaitGroup{}
		wg.Add(3)

		var cumalativeValue int64
		incAt := []time.Duration{100, 200, 500}
		for i := 0; i < 3; i++ {
			incInt := incAt[i]

			go func() {
				t := time.Duration(0)
				for t < 15500 {
					timer := time.NewTicker(incInt * time.Millisecond)
					<-timer.C
					counter.Set(atomic.AddInt64(&cumalativeValue, rand.Int63n(4)+1))
					t += incInt
				}
				wg.Done()
			}()
		}
		wg.Wait()

		msvc.Stop()
		Expect(counter.Get()).To(Equal(atomic.LoadInt64(&cumalativeValue)))
		Expect(s.numEvents).To(Equal(16))
	})
})

type testSender struct {
	events []*cloudevents.Event

	iteration,
	numEvents,
	cumalativeValue int
}
func (s *testSender) PostMeasurementEvents(cloudEvents []*cloudevents.Event) ([]events.CloudEventError, error) {
	defer GinkgoRecover()

	var (
		err error
	)
	s.iteration++

	resp := []events.CloudEventError{}
	if s.iteration > 1 {
		for i, e := range cloudEvents {

			if s.iteration == 2 && i == 1 {
				// fail the second event which should repost next iteration
				resp = append(
					resp,
					events.CloudEventError{
						Event: e,
						Error: fmt.Sprintf("%s failed to post", e.Context.GetID()),
					},
				)
				continue
			}

			Expect(e.Context.GetID()).To(MatchRegexp("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"))
			Expect(e.Context.GetType()).To(Equal("io.appbricks.mycs.network.metric"))
			Expect(e.Context.GetSubject()).To(Equal("Application Monitor Snapshot"))
			Expect(e.Context.GetDataContentType()).To(Equal("application/json"))

			data := make(map[string]interface{})
			err = json.Unmarshal(e.Data(), &data)
			Expect(err).NotTo(HaveOccurred())

			s.numEvents++
			s.cumalativeValue += int((utils.MustGetValueAtPath("monitors/0/counters/0/value", data)).(float64))
			s.events = append(s.events, e)
		}
	} else {
		// first iteration we fail the entire post. counters should repost next iteration
		return nil, fmt.Errorf("failing post")
	}

	return resp, nil
}
