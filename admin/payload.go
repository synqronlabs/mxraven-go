package admin

import (
	"errors"
	"strings"

	"github.com/go-faster/jx"
	"github.com/google/uuid"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

// DeliverPayload returns the empty payload for the DELIVER terminal action.
func DeliverPayload() api.DefaultTerminalActionPayload {
	return api.DefaultTerminalActionPayload{}
}

// DeliverDedicatedPayload returns a DELIVER_DEDICATED payload for a dedicated
// IP pool.
func DeliverDedicatedPayload(poolID uuid.UUID) api.DefaultTerminalActionPayload {
	return api.DefaultTerminalActionPayload{
		"pool_id": rawString(poolID.String()),
	}
}

// RelayPayload returns a RELAY or SMARTHOST_RELAY payload for a relay
// reference.
func RelayPayload(relayRef string) api.DefaultTerminalActionPayload {
	return api.DefaultTerminalActionPayload{
		"relay_ref": rawString(relayRef),
	}
}

// AutoReplyPayload returns an AUTO_REPLY payload for an auto-reply template
// reference.
func AutoReplyPayload(templateRef string) api.DefaultTerminalActionPayload {
	return api.DefaultTerminalActionPayload{
		"template_ref": rawString(templateRef),
	}
}

// DropPayload returns a DROP payload with an optional audit reason.
func DropPayload(auditReason ...string) api.DefaultTerminalActionPayload {
	payload := api.DefaultTerminalActionPayload{}
	if reason := optionalReason(auditReason); reason != "" {
		payload["audit_reason"] = rawString(reason)
	}
	return payload
}

// RejectPayload returns a REJECT payload.
//
// smtpStatusCode must be in the 5xx range. enhancedStatusCode must use the
// class.subject.detail form, for example "5.1.1". message must not be blank.
// auditReason is optional.
func RejectPayload(smtpStatusCode int, enhancedStatusCode, message string, auditReason ...string) (api.DefaultTerminalActionPayload, error) {
	if smtpStatusCode < 500 || smtpStatusCode > 599 {
		return nil, errors.New("admin: SMTP status code must be in the 5xx range")
	}
	enhancedStatusCode = strings.TrimSpace(enhancedStatusCode)
	if !validEnhancedStatusCode(enhancedStatusCode) {
		return nil, errors.New("admin: enhanced status code must use the class.subject.detail form")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, errors.New("admin: rejection message must not be empty")
	}
	payload := api.DefaultTerminalActionPayload{
		"smtp_status_code":     rawInt(smtpStatusCode),
		"enhanced_status_code": rawString(enhancedStatusCode),
		"message":              rawString(message),
	}
	if reason := optionalReason(auditReason); reason != "" {
		payload["audit_reason"] = rawString(reason)
	}
	return payload, nil
}

// optionalReason returns the first non-blank reason, or an empty string.
func optionalReason(reasons []string) string {
	for _, reason := range reasons {
		if reason = strings.TrimSpace(reason); reason != "" {
			return reason
		}
	}
	return ""
}

// validEnhancedStatusCode reports whether value uses the
// class.subject.detail form.
func validEnhancedStatusCode(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// rawString encodes value as a JSON string.
func rawString(value string) jx.Raw {
	var encoder jx.Encoder
	encoder.Str(value)
	return encoder.Bytes()
}

// rawInt encodes value as a JSON number.
func rawInt(value int) jx.Raw {
	var encoder jx.Encoder
	encoder.Int(value)
	return encoder.Bytes()
}
