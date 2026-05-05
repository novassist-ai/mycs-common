package events

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/sirupsen/logrus"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

type PublishDataInput struct {
	Type       string `json:"type"`
	Compressed bool   `json:"compressed"`
	Payload    string `json:"payload"`
}

type PublishEventResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type CloudEventError struct {
	Event *cloudevents.Event
	Error string	
}

func CreatePublishEventList(eventSource string, cloudEvents []*cloudevents.Event) []PublishDataInput {

	var (
		err         error
		dataPayload *PublishDataInput
	)

	dataPayloads := make([]PublishDataInput, 0, len(cloudEvents))
	for _, event := range cloudEvents {
		event.SetSource(eventSource)
		if dataPayload, err = NewPublishDataInput(event); err != nil {
			continue
		}
		dataPayloads = append(dataPayloads, *dataPayload)
	}
	return dataPayloads
}

func MapCloudEventsToPublishDataInputs(cloudEvents []*cloudevents.Event) []PublishDataInput {

	var (
		err         error
		dataPayload *PublishDataInput
	)

	dataPayloads := make([]PublishDataInput, 0, len(cloudEvents))
	for _, event := range cloudEvents {
		if dataPayload, err = NewPublishDataInput(event); err != nil {
			continue
		}
		dataPayloads = append(dataPayloads, *dataPayload)
	}
	return dataPayloads
}

func CreateCloudEventErrorList(publishEventErrorList []PublishEventResult, cloudEvents []*cloudevents.Event) []CloudEventError {

	errors := []CloudEventError{}
	for i, result := range publishEventErrorList {
		if !bool(result.Success) {
			errors = append(errors, CloudEventError{
				Event: cloudEvents[i],
				Error: string(result.Error),
			})
		}
	}
	return errors
}

func FilterCloudEventsWithErrors(publishEventResults []PublishEventResult, cloudEvents []*cloudevents.Event) []*cloudevents.Event {

	eventsWithError := []*cloudevents.Event{}
	for i, result := range publishEventResults {
		cloudEvent := cloudEvents[i]

		if !bool(result.Success) {
			logger.ErrorMessage(
				"FilterCloudEventsWithErrors(): Received error: %s; when publishing cloud event: %s",
				result.Error, cloudEvent.String(),
			)
			eventsWithError = append(eventsWithError, cloudEvents[i])
		}
	}
	return eventsWithError
}

func NewPublishDataInput(event *cloudevents.Event) (*PublishDataInput, error) {

	var (
		err error

		zlibWriter *zlib.Writer

		eventPayload      []byte
		compressedPayload bytes.Buffer
	)

	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		logger.DebugMessage("events.CreatePublishEventList(): Preparing event for posting: %s", event.String())
	}
	
	if eventPayload, err = event.MarshalJSON(); err != nil {
		logger.ErrorMessage("events.CreatePublishEventList(): Unable to marshal event: %s", err.Error())
		return nil, err
	}

	// compress payload and add it to list of payloads
	zlibWriter = zlib.NewWriter(&compressedPayload)
	if _, err = zlibWriter.Write([]byte(eventPayload)); err != nil {
		logger.ErrorMessage("EventsAPI.PostMeasurementEvents(): Unable to compress marshaled event: %s", event.String())
		zlibWriter.Close()
		return nil, err
	}
	zlibWriter.Close()

	return &PublishDataInput{
		Type: "event",
		Compressed: true,
		Payload: base64.StdEncoding.EncodeToString(compressedPayload.Bytes()),
	}, nil
}
