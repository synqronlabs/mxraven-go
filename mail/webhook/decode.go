package webhook

import (
	"encoding/json"
	"fmt"
)

// Decode decodes a webhook payload body.
//
// It dispatches on the payload's event_type field. SMTP delivery statuses do
// not carry an event_type, so a body with a status field and no recognized
// event type is decoded as a [DeliveryStatus].
//
// Decode does not verify the request signature. Call [Verifier.VerifyAndDecode]
// or [Verifier.Verify] first when the payload came from the network.
func Decode(body []byte) (Event, error) {
	var probe struct {
		EventType string         `json:"event_type"`
		Status    *StatusOutcome `json:"status"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return Event{}, fmt.Errorf("webhook: decode payload: %w", err)
	}

	switch probe.EventType {
	case string(EventTypeInboundEmail):
		var payload InboundEmail
		if err := json.Unmarshal(body, &payload); err != nil {
			return Event{}, fmt.Errorf("webhook: decode inbound email payload: %w", err)
		}
		return Event{Type: EventTypeInboundEmail, InboundEmail: &payload}, nil
	case string(EventTypeStorageStatus):
		var payload StorageStatus
		if err := json.Unmarshal(body, &payload); err != nil {
			return Event{}, fmt.Errorf("webhook: decode storage status payload: %w", err)
		}
		return Event{Type: EventTypeStorageStatus, StorageStatus: &payload}, nil
	}

	if probe.Status != nil {
		var payload DeliveryStatus
		if err := json.Unmarshal(body, &payload); err != nil {
			return Event{}, fmt.Errorf("webhook: decode delivery status payload: %w", err)
		}
		return Event{Type: EventTypeDeliveryStatus, DeliveryStatus: &payload}, nil
	}

	return Event{}, fmt.Errorf("webhook: unrecognized payload: event_type %q", probe.EventType)
}
