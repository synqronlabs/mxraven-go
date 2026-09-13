package webhook_test

import (
	"testing"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

func TestDecodeInboundEmail(t *testing.T) {
	event, err := webhook.Decode([]byte(inboundBody))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if event.Type != webhook.EventTypeInboundEmail {
		t.Errorf("Type = %q", event.Type)
	}
	if event.InboundEmail == nil {
		t.Fatal("InboundEmail = nil")
	}
	if event.InboundEmail.Envelope.MailFrom != "sender@example.net" {
		t.Errorf("MailFrom = %q", event.InboundEmail.Envelope.MailFrom)
	}
	if len(event.InboundEmail.Headers) != 1 || event.InboundEmail.Headers[0].Name != "From" {
		t.Errorf("Headers = %+v", event.InboundEmail.Headers)
	}
	if event.InboundEmail.RawEmail.AccessToken != "token" {
		t.Errorf("AccessToken = %q", event.InboundEmail.RawEmail.AccessToken)
	}
}

func TestDecodeDeliveryStatus(t *testing.T) {
	body := `{
		"task_id": "task-1",
		"tenant_id": "tenant-1",
		"listener_id": "listener-1",
		"status": "deferred",
		"attempt": 2,
		"accepted_at_utc": 1789302600,
		"occurred_at_utc": 1789302700,
		"smtp_code": 451,
		"enhanced_status_code": "4.7.1",
		"remote_response": "greylisted"
	}`
	event, err := webhook.Decode([]byte(body))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if event.Type != webhook.EventTypeDeliveryStatus {
		t.Errorf("Type = %q", event.Type)
	}
	if event.DeliveryStatus == nil {
		t.Fatal("DeliveryStatus = nil")
	}
	if event.DeliveryStatus.Status != webhook.StatusDeferred {
		t.Errorf("Status = %q", event.DeliveryStatus.Status)
	}
	if event.DeliveryStatus.SMTPCode != 451 {
		t.Errorf("SMTPCode = %d", event.DeliveryStatus.SMTPCode)
	}
}

func TestDecodeStorageStatus(t *testing.T) {
	body := `{
		"event_type": "s3_egress_status",
		"task_id": "task-1",
		"tenant_id": "tenant-1",
		"listener_id": "listener-1",
		"status": "delivered",
		"attempt": 1,
		"occurred_at_utc": 1789302700,
		"storage_ref": "primary",
		"bucket_name": "mail",
		"object_key": "2026/09/task-1.eml"
	}`
	event, err := webhook.Decode([]byte(body))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if event.Type != webhook.EventTypeStorageStatus {
		t.Errorf("Type = %q", event.Type)
	}
	if event.StorageStatus == nil {
		t.Fatal("StorageStatus = nil")
	}
	if event.StorageStatus.Status != webhook.StatusDelivered {
		t.Errorf("Status = %q", event.StorageStatus.Status)
	}
	if event.StorageStatus.ObjectKey != "2026/09/task-1.eml" {
		t.Errorf("ObjectKey = %q", event.StorageStatus.ObjectKey)
	}
}

func TestDecodeInvalid(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not json", "{"},
		{"unknown type", `{"event_type": "something_else"}`},
		{"empty", ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := webhook.Decode([]byte(tt.body)); err == nil {
				t.Fatal("Decode() error = nil, want error")
			}
		})
	}
}
