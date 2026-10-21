package notify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

// Kind identifies which kind of notification an [Event] carries.
type Kind string

const (
	// KindAccepted is a message the listener accepted and whose terminal
	// action was resolved.
	KindAccepted Kind = "accepted"
	// KindAttempt is one delivery attempt for an accepted message on an egress
	// channel.
	KindAttempt Kind = "attempt"
)

// Channel identifies the egress worker that produced an [AttemptEvent].
type Channel string

const (
	// ChannelSMTP is an SMTP delivery attempt.
	ChannelSMTP Channel = "smtp"
	// ChannelS3 is an object-storage write attempt.
	ChannelS3 Channel = "s3"
	// ChannelWebhook is a webhook delivery attempt.
	ChannelWebhook Channel = "webhook"
)

// Event is a decoded client notification. Exactly one of the payload fields is
// non-nil, matching Kind.
type Event struct {
	// Kind identifies the payload shape.
	Kind Kind
	// Accepted is set for an accepted-message event. It is the same payload a
	// DELIVER_WEBHOOK delivery carries.
	Accepted *webhook.InboundEmail
	// Attempt is set for a delivery-attempt event.
	Attempt *AttemptEvent
}

// AttemptEvent is one delivery attempt for an accepted message. The Channel
// field names which worker produced it; the remaining fields describe that
// channel's protocol and are zero when they do not apply.
//
// A rule that chains side-effect actions reports each action it triggers, so a
// message can produce more than one attempt event. Every attempt carries a
// CorrelationID that points at the TaskID of the accepted event for the message
// it stems from, which is how an attempt joins back to its acceptance.
type AttemptEvent struct {
	// EventType is the producer's event-type marker when one is set. SMTP
	// attempt events omit it.
	EventType string `json:"event_type,omitempty"`
	// Channel names the egress worker that produced the attempt.
	Channel Channel `json:"channel"`
	// TaskID identifies the delivery task. For a submission listener it equals
	// the accepted event's TaskID; otherwise use CorrelationID to reach it.
	TaskID string `json:"task_id,omitempty"`
	// TenantID is the owning tenant.
	TenantID string `json:"tenant_id,omitempty"`
	// ListenerID is the source listener.
	ListenerID string `json:"listener_id,omitempty"`
	// MessageRef is the mxRaven message reference of the message this attempt
	// belongs to. It joins attempts and acceptance events for the same message,
	// including every recipient a single send fanned out to.
	MessageRef string `json:"message_ref,omitempty"`
	// CorrelationID is the TaskID of the accepted event for the message this
	// attempt belongs to, when one was recorded.
	CorrelationID string `json:"correlation_id,omitempty"`
	// Status is the attempt outcome.
	Status webhook.StatusOutcome `json:"status"`
	// Attempt is the delivery attempt number, starting at 1.
	Attempt uint64 `json:"attempt"`
	// AcceptedAtUTC is the Unix time, in seconds, when mxRaven accepted the
	// message. It is zero when the producer does not report it.
	AcceptedAtUTC int64 `json:"accepted_at_utc,omitempty"`
	// OccurredAtUTC is the Unix time, in seconds, when the attempt was built.
	OccurredAtUTC int64 `json:"occurred_at_utc"`

	// SourceIP is the SMTP egress source address, when known.
	SourceIP string `json:"source_ip,omitempty"`
	// DestinationDomain is the recipient domain for an SMTP attempt.
	DestinationDomain string `json:"destination_domain,omitempty"`
	// RemoteHost is the remote MTA hostname, when known.
	RemoteHost string `json:"remote_host,omitempty"`
	// SMTPCode is the remote SMTP reply code.
	SMTPCode int `json:"smtp_code,omitempty"`
	// EnhancedStatusCode is the RFC 3463 enhanced status code.
	EnhancedStatusCode string `json:"enhanced_status_code,omitempty"`
	// Reason is the bounded, machine-readable delivery classification. See the
	// SMTP egress documentation for the value set.
	Reason string `json:"reason,omitempty"`
	// RemoteResponse is the remote reply text, truncated by the server.
	RemoteResponse string `json:"remote_response,omitempty"`
	// CorrelationTaskID links a generated DSN back to its original task.
	CorrelationTaskID string `json:"correlation_task_id,omitempty"`

	// StorageRef identifies the storage integration for an s3 attempt.
	StorageRef string `json:"storage_ref,omitempty"`
	// BucketName is the destination bucket for an s3 attempt.
	BucketName string `json:"bucket_name,omitempty"`
	// ObjectKey is the stored object key for an s3 attempt.
	ObjectKey string `json:"object_key,omitempty"`
	// EndpointHost is the storage endpoint host for an s3 attempt.
	EndpointHost string `json:"endpoint_host,omitempty"`
	// ErrorCode is the provider error code, when one was returned.
	ErrorCode string `json:"error_code,omitempty"`

	// StatusCode is the storage or webhook HTTP response status.
	StatusCode int `json:"status_code,omitempty"`
	// Message describes the failure, truncated by the server.
	Message string `json:"message,omitempty"`
	// NextRetryAtUTC is the Unix time, in seconds, of the next attempt when the
	// attempt will be retried.
	NextRetryAtUTC int64 `json:"next_retry_at_utc,omitempty"`

	// RetryAfterMS is the webhook retry-after hint, in milliseconds.
	RetryAfterMS int64 `json:"retry_after_ms,omitempty"`
	// TargetURLHost is the webhook target host.
	TargetURLHost string `json:"target_url_host,omitempty"`
	// WebhookRef identifies the webhook endpoint.
	WebhookRef string `json:"webhook_ref,omitempty"`
	// SigningKeyKID is the key ID used to sign the webhook attempt.
	SigningKeyKID string `json:"signing_key_kid,omitempty"`
	// RawTokenTTLSeconds is the raw-message token lifetime the webhook attempt
	// used.
	RawTokenTTLSeconds int `json:"raw_token_ttl_seconds,omitempty"`
}

// DecodeError reports an event body that could not be decoded. It is returned
// when a notification does not match the SDK's payload model, which normally
// means the SDK is older than the server that produced it.
type DecodeError struct {
	// Subject is the notification subject the body was published to.
	Subject string
	// Err is the underlying decode failure.
	Err error
}

// Error implements the error interface.
func (e *DecodeError) Error() string {
	return fmt.Sprintf("notify: decode %q: %v", e.Subject, e.Err)
}

// Unwrap returns the underlying decode failure.
func (e *DecodeError) Unwrap() error { return e.Err }

// decodeEvent decodes one notification body. The event kind is the final token
// of the subject; when it is not recognized, the body is probed instead.
func decodeEvent(subject string, body []byte) (Event, error) {
	switch kind := subjectKind(subject); kind {
	case string(KindAccepted):
		return decodeAccepted(subject, body)
	case string(KindAttempt):
		return decodeAttempt(subject, body)
	}
	switch probeKind(body) {
	case KindAccepted:
		return decodeAccepted(subject, body)
	case KindAttempt:
		return decodeAttempt(subject, body)
	default:
		return Event{}, &DecodeError{Subject: subject, Err: fmt.Errorf("unrecognized notification body")}
	}
}

func decodeAccepted(subject string, body []byte) (Event, error) {
	var payload webhook.InboundEmail
	if err := json.Unmarshal(body, &payload); err != nil {
		return Event{}, &DecodeError{Subject: subject, Err: fmt.Errorf("accepted event: %w", err)}
	}
	return Event{Kind: KindAccepted, Accepted: &payload}, nil
}

func decodeAttempt(subject string, body []byte) (Event, error) {
	var payload AttemptEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		return Event{}, &DecodeError{Subject: subject, Err: fmt.Errorf("attempt event: %w", err)}
	}
	return Event{Kind: KindAttempt, Attempt: &payload}, nil
}

// subjectKind returns the final subject token, which is the event kind.
func subjectKind(subject string) string {
	subject = strings.TrimSpace(subject)
	if i := strings.LastIndexByte(subject, '.'); i >= 0 {
		return strings.TrimSpace(subject[i+1:])
	}
	return subject
}

// probeKind infers the event kind from a body whose subject is unavailable or
// unrecognized.
func probeKind(body []byte) Kind {
	var probe struct {
		EventType string `json:"event_type"`
		Channel   string `json:"channel"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	switch {
	case probe.EventType == string(webhook.EventTypeInboundEmail):
		return KindAccepted
	case probe.Channel != "":
		return KindAttempt
	default:
		return ""
	}
}
