package mail

import (
	"testing"

	ravenclient "github.com/synqronlabs/raven/client"
)

func TestParseMessageRef(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "submission reply",
			message: "2.0.0 accepted; message_ref=86a33087-51ab-40a2-a020-d2745fe08d34",
			want:    "86a33087-51ab-40a2-a020-d2745fe08d34",
		},
		{
			name:    "angle bracketed",
			message: "2.0.0 accepted; message_ref=<abc-123>",
			want:    "abc-123",
		},
		{
			name:    "trailing semicolon",
			message: "2.0.0 accepted; message_ref=abc; extra",
			want:    "abc",
		},
		{
			name:    "missing",
			message: "2.0.0 accepted",
			want:    "",
		},
		{
			name:    "empty",
			message: "",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMessageRef(tt.message); got != tt.want {
				t.Errorf("parseMessageRef(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

func TestResultFrom(t *testing.T) {
	if got := resultFrom(nil); got != nil {
		t.Fatalf("resultFrom(nil) = %v, want nil", got)
	}

	res := &ravenclient.SendResult{
		Success:   true,
		MessageID: "",
		Response: &ravenclient.ClientResponse{
			Code:    250,
			Message: "2.0.0 accepted; message_ref=ref-1",
		},
		RecipientResults: []ravenclient.RecipientResult{
			{
				Address:  "a@example.com",
				Accepted: true,
			},
			{
				Address:  "b@example.com",
				Accepted: false,
				Error:    &ravenclient.SMTPError{Code: 550, EnhancedCode: "5.1.1", Message: "no such user"},
			},
		},
	}

	result := resultFrom(res)
	if result == nil {
		t.Fatal("resultFrom() = nil")
	}
	if result.MessageRef != "ref-1" {
		t.Errorf("MessageRef = %q, want ref-1", result.MessageRef)
	}
	if result.Code != 250 {
		t.Errorf("Code = %d, want 250", result.Code)
	}
	if len(result.Recipients) != 2 {
		t.Fatalf("len(Recipients) = %d, want 2", len(result.Recipients))
	}
	if !result.Recipients[0].Accepted {
		t.Error("recipient 0 not accepted")
	}
	smtpErr, ok := result.Recipients[1].Error.(*SMTPError)
	if !ok {
		t.Fatalf("recipient 1 error = %T, want *SMTPError", result.Recipients[1].Error)
	}
	if !smtpErr.Permanent() {
		t.Error("recipient 1 error is not permanent")
	}
}
