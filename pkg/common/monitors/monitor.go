package monitors

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/novassist/mycs-common/pkg/common/events"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/uuid"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
)

const networkMetricEventType = `io.appbricks.mycs.network.metric`

type Sender interface {
	PostMeasurementEvents(events []*cloudevents.Event) ([]events.CloudEventError, error)
}

type Monitor struct {
	name     string
	counters []*Counter

	lock *sync.Mutex
}

type MonitorService struct {
	ctx    context.Context
	cancel context.CancelFunc

	sender Sender
	sendWG sync.WaitGroup

	collectInterval time.Duration

	collectCount,
	sendCountdown int

	monitors []*Monitor
	lock     sync.Mutex

	eventPayloads []*eventPayload

	snapshotTimer *utils.ExecTimer
}

type eventPayload struct {
	Monitors []*monitorSnapshot `json:"monitors"`
}
type monitorSnapshot struct {
	Name     string             `json:"name"`
	Counters []*counterSnapshot `json:"counters"`
}

// Creates a new monitor services with a 'sender' that
// will post monitor events to an upstream service. The 
// monitor collects metrics from all counters every
// 'collectInterval' milliseconds and publishes these
// metrics after 'collectCount' collections.
func NewMonitorService(sender Sender, collectCount, collectInterval int) *MonitorService {

	ctx, cancel := context.WithCancel(context.Background())

	return &MonitorService{
		ctx:    ctx,
		cancel: cancel,

		sender:          sender,
		collectInterval: time.Duration(collectInterval),
		collectCount:    collectCount-1,
		sendCountdown:   collectCount-1,

		monitors: []*Monitor{},

		// payload for each snapshot collected
		eventPayloads: make([]*eventPayload, 0, collectCount),
	}
}

func (ms *MonitorService) NewMonitor(name string) *Monitor {
	ms.lock.Lock()
	defer ms.lock.Unlock()

	monitor := &Monitor{
		name:     name,
		counters: []*Counter{},

		lock: &ms.lock,
	}
	ms.monitors = append(ms.monitors, monitor)

	return monitor
}

func (ms *MonitorService) Start() error {
	ms.snapshotTimer = utils.NewExecTimer(ms.ctx, ms.collect, false)
	return ms.snapshotTimer.Start(ms.collectInterval)
}

func (ms *MonitorService) collect() (time.Duration, error) {
	ms.lock.Lock()
	defer ms.lock.Unlock()

	ms.collectEvents()
	if ms.sendCountdown == 0 {
		ms.postEvents()
		ms.sendCountdown = ms.collectCount
	} else {
		ms.sendCountdown--
	}

	// metrics collected every second
	return ms.collectInterval, nil
}

func (ms *MonitorService) collectEvents() {

	addPayload := false
	eventPayload := eventPayload{}
	for _, m := range ms.monitors {
		if len(m.counters) > 0 {
			monitorSnapshot := monitorSnapshot{
				Name: m.name,
			}
			eventPayload.Monitors = append(eventPayload.Monitors, &monitorSnapshot)
	
			for _, c := range m.counters {
				counterSnapshot := c.collect()
				if counterSnapshot != nil {
					monitorSnapshot.Counters = append(monitorSnapshot.Counters, counterSnapshot)
					addPayload = true	
				}
			}	
		}
	}
	if addPayload {
		ms.eventPayloads = append(ms.eventPayloads, &eventPayload)
	}
}

func (ms *MonitorService) postEvents() {
	numEvents := len(ms.eventPayloads)

	// make a copy of all the payloads that will
	// be pushed to the cloud asynchronously
	eventPayloads := make([]*eventPayload, numEvents)
	copy(eventPayloads, ms.eventPayloads)
	ms.eventPayloads = ms.eventPayloads[:0]

	ms.sendWG.Add(1)
	go func() {
		defer ms.sendWG.Done()

		var (
			err error

			postEventErrors []events.CloudEventError
		)

		events := make([]*event.Event, 0, numEvents)
		for _, data := range eventPayloads {
			eventUUID := uuid.NewString()

			event := cloudevents.NewEvent()
			event.SetID(eventUUID)
			event.SetType(networkMetricEventType)
			event.SetSource("urn:mycs")
			event.SetSubject("Application Monitor Snapshot")
			event.SetDataContentType("application/json")
			event.SetTime(time.Now())
			if err = event.SetData(cloudevents.ApplicationJSON, data); err != nil {
				logger.ErrorMessage(
					"monitorService.postEvents(): Unable to add monitor payload to cloud event instance with id \"%s\": %s",
					eventUUID, err.Error(),
				)
			}
			events = append(events, &event)
		}
		if len(events) > 0 {
			if postEventErrors, err = ms.sender.PostMeasurementEvents(events); err != nil {
				logger.ErrorMessage(
					"monitorService.postEvents(): Unable to post measurement events. Will attempt to re-post in next cycle: %s",
					err.Error(),
				)
				// put back the counters
				ms.lock.Lock()
				ms.eventPayloads = append(eventPayloads, ms.eventPayloads...)
				ms.lock.Unlock()

			} else if len(postEventErrors) > 0 {
				repostList := []*eventPayload{}
				for _, e := range postEventErrors {
					logger.ErrorMessage(
						"monitorService.postEvents(): Event with id %s failed to post with error: %s",
						e.Event.Context.GetID(), e.Error,
					)
					ep := new(eventPayload)
					if err = json.Unmarshal(e.Event.Data(), &ep); err != nil {
						logger.ErrorMessage(
							"monitorService.postEvents(): Unable to unmarshal data for event with id %s to queue for reposting: %s",
							e.Event.Context.GetID(), err.Error(),
						)
					} else {
						repostList = append(repostList, ep)
					}
				}
				// put back counters that were not pushed to the event bus
				ms.lock.Lock()
				ms.eventPayloads = append(repostList, ms.eventPayloads...)
				ms.lock.Unlock()
			}
		}
	}()
}

func (ms *MonitorService) Stop() {

	if ms.snapshotTimer != nil {
		if err := ms.snapshotTimer.Stop(); err != nil {
			logger.DebugMessage(
				"monitorService.Stop(): Auth execution timer stopped with err: %s",
				err.Error())
		}
	}
	ms.sendWG.Wait()

	// ensure all data that is waiting to
	// be collected or posted are processed
	ms.lock.Lock()
	ms.collectEvents()
	ms.postEvents()
	ms.lock.Unlock()
	ms.sendWG.Wait()
}

func (m *Monitor) AddCounter(counter *Counter) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.counters = append(m.counters, counter)
}

func (m *Monitor) DeleteCounter(counter *Counter) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// ref: https://go.dev/play/p/6D7u9x9UNw2
	j := len(m.counters) - 1
	for i := j; i >= 0; i-- {
		if m.counters[i] == counter {
			copy(m.counters[i:], m.counters[i+1:])
			break
		}
	}
	m.counters = m.counters[:j]
}
