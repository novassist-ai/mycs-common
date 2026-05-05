package events_test

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/novassist/mycs-common/pkg/common/events"
	cloudevents "github.com/cloudevents/sdk-go/v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Events", func() {

	It("create list of events to publish", func() {

		var (
			err error

			b []byte
			r io.Reader
			data strings.Builder
		)
	
		cloudEvents := []*cloudevents.Event{}
		for _, e := range testEvents {
			event := cloudevents.NewEvent()
			err = json.Unmarshal([]byte(e), &event)
			Expect(err).NotTo(HaveOccurred())
			cloudEvents = append(cloudEvents, &event)
		}

		publishEventList := events.CreatePublishEventList("urn:mycs:device:12345", cloudEvents)
		Expect(len(publishEventList)).To(Equal(len(testEvents)))

		for i, publishEvent := range publishEventList {
			data.Reset()

			cloudEventData := testEvents[i]
			Expect(publishEvent.Type).To(Equal("event"))
			Expect(publishEvent.Compressed).To(Equal(true))

			b, err = base64.StdEncoding.DecodeString(publishEvent.Payload)
			Expect(err).NotTo(HaveOccurred())
			r, err = zlib.NewReader(bytes.NewBuffer(b))
			Expect(err).NotTo(HaveOccurred())
			_, err = io.Copy(&data, r)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.String()).To(Equal(cloudEventData))
		}
	})
})

var testEvents = []string{
	`{"specversion":"1.0","id":"441d7a42-06b2-4a23-84a3-85b08dc3c28a","source":"urn:mycs:device:12345","type":"io.appbricks.mycs.network.metric","subject":"Application Monitor Snapshot","datacontenttype":"application/json","time":"2021-12-27T23:41:30.859185Z","data":{"monitors":[{"name":"testMonitor","counters":[{"name":"testCounter","timestamp":1640648486858,"value":32}]}]}}`,
	`{"specversion":"1.0","id":"b77ab608-83ed-404e-9d12-9d0fb6eda3a1","source":"urn:mycs:device:12345","type":"io.appbricks.mycs.network.metric","subject":"Application Monitor Snapshot","datacontenttype":"application/json","time":"2021-12-27T23:41:30.85952Z","data":{"monitors":[{"name":"testMonitor","counters":[{"name":"testCounter","timestamp":1640648487858,"value":42}]}]}}`,
	`{"specversion":"1.0","id":"45d4c35f-cb7e-4cee-ace0-1c2ddfe15e4c","source":"urn:mycs:device:12345","type":"io.appbricks.mycs.network.metric","subject":"Application Monitor Snapshot","datacontenttype":"application/json","time":"2021-12-27T23:41:30.859527Z","data":{"monitors":[{"name":"testMonitor","counters":[{"name":"testCounter","timestamp":1640648488858,"value":52}]}]}}`,
	`{"specversion":"1.0","id":"49504010-9afa-4c3f-b0b8-bef2cc71d4e2","source":"urn:mycs:device:12345","type":"io.appbricks.mycs.network.metric","subject":"Application Monitor Snapshot","datacontenttype":"application/json","time":"2021-12-27T23:41:30.859533Z","data":{"monitors":[{"name":"testMonitor","counters":[{"name":"testCounter","timestamp":1640648489858,"value":38}]}]}}`,
	`{"specversion":"1.0","id":"9315ba87-959a-447c-8946-dde357fbc0b2","source":"urn:mycs:device:12345","type":"io.appbricks.mycs.network.metric","subject":"Application Monitor Snapshot","datacontenttype":"application/json","time":"2021-12-27T23:41:30.859538Z","data":{"monitors":[{"name":"testMonitor","counters":[{"name":"testCounter","timestamp":1640648490859,"value":47}]}]}}`,
}
