package notify

import (
	"errors"
	"strings"
	"testing"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

const (
	acceptedBody = `{
		"event_type": "inbound_email",
		"task_id": "task-1",
		"tenant_id": "tenant-1",
		"listener_id": "listener-1",
		"attempt": 1,
		"accepted_at_utc": 1700000000,
		"occurred_at_utc": 1700000001,
		"routing_decision": {"terminal_action": "TERMINAL_ACTION_TYPE_DELIVER_WEBHOOK", "used_listener_default": false},
		"envelope": {"mail_from": "sender@example.com", "rcpt_to": ["rcpt@example.com"]},
		"message": {"subject": "hi"},
		"headers": [{"name": "Subject", "value": "hi"}],
		"raw_email": {
			"url": "https://raw.example/messages/task-1.eml",
			"token_type": "Bearer",
			"access_token": "secret-token",
			"expires_at_utc": 1700100000,
			"size_bytes": 12,
			"sha256_hex": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
			"content_type": "text/plain"
		}
	}`

	smtpAttemptBody = `{
		"channel": "smtp",
		"task_id": "task-1",
		"tenant_id": "tenant-1",
		"listener_id": "listener-1",
		"message_ref": "message-ref-1",
		"correlation_id": "accepted-1",
		"status": "deferred",
		"attempt": 3,
		"accepted_at_utc": 1700000000,
		"occurred_at_utc": 1700000001,
		"source_ip": "192.0.2.1",
		"destination_domain": "example.com",
		"remote_host": "mx.example.com",
		"smtp_code": 451,
		"enhanced_status_code": "4.7.1",
		"reason": "smtp_transient",
		"remote_response": "try later",
		"next_retry_at_utc": 1700000060,
		"correlation_task_id": "corr-1"
	}`

	s3AttemptBody = `{
		"event_type": "s3_egress_status",
		"channel": "s3",
		"task_id": "task-1",
		"tenant_id": "tenant-1",
		"listener_id": "listener-1",
		"message_ref": "message-ref-1",
		"correlation_id": "accepted-1",
		"status": "delivered",
		"attempt": 2,
		"accepted_at_utc": 1700000000,
		"occurred_at_utc": 1700000001,
		"storage_ref": "storage-1",
		"bucket_name": "bucket",
		"object_key": "inbound/a.eml",
		"endpoint_host": "s3.example.com",
		"status_code": 200
	}`

	webhookAttemptBody = `{
		"event_type": "webhook_egress_status",
		"channel": "webhook",
		"task_id": "task-1",
		"tenant_id": "tenant-1",
		"listener_id": "listener-1",
		"message_ref": "message-ref-1",
		"correlation_id": "accepted-1",
		"status": "failed",
		"attempt": 1,
		"status_code": 500,
		"message": "boom",
		"retry_after_ms": 1000,
		"occurred_at_utc": 1700000001,
		"target_url_host": "hooks.example.com",
		"webhook_ref": "hook-1",
		"signing_key_kid": "kid-1",
		"raw_token_ttl_seconds": 90000
	}`
)

func TestDecodeAccepted(t *testing.T) {
	event, err := decodeEvent("tenant.t.listener.l.events.accepted", []byte(acceptedBody))
	if err != nil {
		t.Fatalf("decodeEvent() error = %v", err)
	}
	if event.Kind != KindAccepted {
		t.Fatalf("Kind = %q, want %q", event.Kind, KindAccepted)
	}
	if event.Accepted == nil || event.Attempt != nil {
		t.Fatalf("payloads = accepted %v, attempt %v; want accepted only", event.Accepted, event.Attempt)
	}
	if got, want := event.Accepted.TaskID, "task-1"; got != want {
		t.Errorf("TaskID = %q, want %q", got, want)
	}
	if got, want := event.Accepted.RawEmail.AccessToken, "secret-token"; got != want {
		t.Errorf("RawEmail.AccessToken = %q, want %q", got, want)
	}
}

func TestDecodeAttemptChannels(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		channel Channel
		status  webhook.StatusOutcome
	}{
		{name: "smtp", body: smtpAttemptBody, channel: ChannelSMTP, status: webhook.StatusDeferred},
		{name: "s3", body: s3AttemptBody, channel: ChannelS3, status: webhook.StatusDelivered},
		{name: "webhook", body: webhookAttemptBody, channel: ChannelWebhook, status: webhook.StatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := decodeEvent("tenant.t.listener.l.events.attempt", []byte(tt.body))
			if err != nil {
				t.Fatalf("decodeEvent() error = %v", err)
			}
			if event.Kind != KindAttempt {
				t.Fatalf("Kind = %q, want %q", event.Kind, KindAttempt)
			}
			if event.Attempt == nil || event.Accepted != nil {
				t.Fatalf("payloads = accepted %v, attempt %v; want attempt only", event.Accepted, event.Attempt)
			}
			if got := event.Attempt.Channel; got != tt.channel {
				t.Errorf("Channel = %q, want %q", got, tt.channel)
			}
			if got := event.Attempt.Status; got != tt.status {
				t.Errorf("Status = %q, want %q", got, tt.status)
			}
			if got, want := event.Attempt.CorrelationID, "accepted-1"; got != want {
				t.Errorf("CorrelationID = %q, want %q", got, want)
			}
			if got, want := event.Attempt.MessageRef, "message-ref-1"; got != want {
				t.Errorf("MessageRef = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeAttemptChannelDetails(t *testing.T) {
	event, err := decodeEvent("x.events.attempt", []byte(smtpAttemptBody))
	if err != nil {
		t.Fatalf("decodeEvent() error = %v", err)
	}
	attempt := event.Attempt
	if got, want := attempt.RemoteHost, "mx.example.com"; got != want {
		t.Errorf("RemoteHost = %q, want %q", got, want)
	}
	if got, want := attempt.SMTPCode, 451; got != want {
		t.Errorf("SMTPCode = %d, want %d", got, want)
	}
	if got, want := attempt.NextRetryAtUTC, int64(1700000060); got != want {
		t.Errorf("NextRetryAtUTC = %d, want %d", got, want)
	}

	event, err = decodeEvent("x.events.attempt", []byte(webhookAttemptBody))
	if err != nil {
		t.Fatalf("decodeEvent() error = %v", err)
	}
	if got, want := event.Attempt.RetryAfterMS, int64(1000); got != want {
		t.Errorf("RetryAfterMS = %d, want %d", got, want)
	}
	if got, want := event.Attempt.SigningKeyKID, "kid-1"; got != want {
		t.Errorf("SigningKeyKID = %q, want %q", got, want)
	}
}

func TestDecodeFallsBackToBodyShape(t *testing.T) {
	for name, body := range map[string]string{
		"accepted": acceptedBody,
		"attempt":  s3AttemptBody,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeEvent("", []byte(body)); err != nil {
				t.Fatalf("decodeEvent() error = %v", err)
			}
		})
	}
}

func TestDecodeInvalidBody(t *testing.T) {
	_, err := decodeEvent("x.events.accepted", []byte("{"))
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("error = %v, want *DecodeError", err)
	}
	if decodeErr.Subject != "x.events.accepted" {
		t.Errorf("Subject = %q, want %q", decodeErr.Subject, "x.events.accepted")
	}
}

func TestDecodeUnrecognizedBody(t *testing.T) {
	_, err := decodeEvent("x.events.unknown", []byte(`{"foo":"bar"}`))
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("error = %v, want *DecodeError", err)
	}
}

func TestDecodeErrorMethods(t *testing.T) {
	cause := errors.New("cause")
	decodeErr := &DecodeError{Subject: "tenant.t.listener.l.events.accepted", Err: cause}
	if got := decodeErr.Error(); !strings.Contains(got, "accepted") || !strings.Contains(got, "cause") {
		t.Fatalf("Error() = %q, want subject and cause", got)
	}
	if !errors.Is(decodeErr, cause) {
		t.Fatal("errors.Is() = false, want true")
	}
}

func TestSubjectKind(t *testing.T) {
	if got, want := subjectKind("tenant.t.listener.l.events.accepted"), "accepted"; got != want {
		t.Errorf("subjectKind() = %q, want %q", got, want)
	}
	if got, want := subjectKind("  spaced.attempt  "), "attempt"; got != want {
		t.Errorf("subjectKind() = %q, want %q", got, want)
	}
	if got, want := subjectKind("noseparator"), "noseparator"; got != want {
		t.Errorf("subjectKind() = %q, want %q", got, want)
	}
}
