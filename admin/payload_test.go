package admin_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/synqronlabs/mxraven-go/admin"
)

func TestTerminalActionPayloads(t *testing.T) {
	poolID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	if payload := admin.DeliverPayload(); payload == nil || len(payload) != 0 {
		t.Errorf("DeliverPayload() = %v, want empty non-nil payload", payload)
	}
	if got, want := string(admin.DeliverDedicatedPayload(poolID)["pool_id"]), `"`+poolID.String()+`"`; got != want {
		t.Errorf("pool_id = %s, want %s", got, want)
	}
	if got, want := string(admin.RelayPayload("relay-1")["relay_ref"]), `"relay-1"`; got != want {
		t.Errorf("relay_ref = %s, want %s", got, want)
	}
	if got, want := string(admin.AutoReplyPayload("template-1")["template_ref"]), `"template-1"`; got != want {
		t.Errorf("template_ref = %s, want %s", got, want)
	}
	if payload := admin.DropPayload(); len(payload) != 0 {
		t.Errorf("DropPayload() = %v, want empty payload", payload)
	}
	if got, want := string(admin.DropPayload("policy")["audit_reason"]), `"policy"`; got != want {
		t.Errorf("audit_reason = %s, want %s", got, want)
	}
}

func TestRejectPayload(t *testing.T) {
	payload, err := admin.RejectPayload(550, "5.1.1", "No such user", "policy")
	if err != nil {
		t.Fatalf("RejectPayload() error: %v", err)
	}
	if got, want := string(payload["smtp_status_code"]), "550"; got != want {
		t.Errorf("smtp_status_code = %s, want %s", got, want)
	}
	if got, want := string(payload["enhanced_status_code"]), `"5.1.1"`; got != want {
		t.Errorf("enhanced_status_code = %s, want %s", got, want)
	}
	if got, want := string(payload["message"]), `"No such user"`; got != want {
		t.Errorf("message = %s, want %s", got, want)
	}
	if got, want := string(payload["audit_reason"]), `"policy"`; got != want {
		t.Errorf("audit_reason = %s, want %s", got, want)
	}

	withoutReason, err := admin.RejectPayload(550, "5.1.1", "No such user")
	if err != nil {
		t.Fatalf("RejectPayload() error: %v", err)
	}
	if _, ok := withoutReason["audit_reason"]; ok {
		t.Error("audit_reason present, want omitted")
	}
}

func TestRejectPayloadValidation(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		enhanced   string
		message    string
	}{
		{name: "status below 5xx", statusCode: 499, enhanced: "5.1.1", message: "no"},
		{name: "status above 5xx", statusCode: 600, enhanced: "5.1.1", message: "no"},
		{name: "malformed enhanced code", statusCode: 550, enhanced: "5.1", message: "no"},
		{name: "non-numeric enhanced code", statusCode: 550, enhanced: "5.1.a", message: "no"},
		{name: "blank message", statusCode: 550, enhanced: "5.1.1", message: "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := admin.RejectPayload(tt.statusCode, tt.enhanced, tt.message); err == nil {
				t.Fatal("RejectPayload() error = nil, want validation error")
			}
		})
	}
}
