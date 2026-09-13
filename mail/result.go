package mail

import (
	"strings"

	ravenclient "github.com/synqronlabs/raven/client"
)

// Result describes the outcome of a successful submission.
type Result struct {
	// MessageRef is the mxRaven message reference assigned to the submission,
	// as returned in the server's final "message_ref=<uuid>" reply. It is
	// empty when the server did not report one.
	MessageRef string
	// Code and Message are the final SMTP reply. Message is intended for
	// humans and is not stable.
	Code    int
	Message string
	// Recipients reports per-recipient acceptance. It is populated when the
	// server returned individual RCPT responses.
	Recipients []RecipientResult
}

// RecipientResult reports whether a single envelope recipient was accepted.
type RecipientResult struct {
	// Address is the recipient address as submitted.
	Address string
	// Accepted reports whether the server accepted the recipient.
	Accepted bool
	// Error is the rejection reason when Accepted is false.
	Error error
}

// resultFrom converts a raven send result into a [Result]. It returns nil when
// res is nil, which happens when the transaction failed before any reply was
// recorded.
func resultFrom(res *ravenclient.SendResult) *Result {
	if res == nil {
		return nil
	}
	result := &Result{}
	if res.Response != nil {
		result.Code = res.Response.Code
		result.Message = res.Response.Message
		result.MessageRef = parseMessageRef(res.Response.Message)
	}
	if len(res.RecipientResults) > 0 {
		result.Recipients = make([]RecipientResult, len(res.RecipientResults))
		for i, recipient := range res.RecipientResults {
			result.Recipients[i] = RecipientResult{
				Address:  recipient.Address,
				Accepted: recipient.Accepted,
				Error:    translateSMTPError(recipient.Error),
			}
		}
	}
	return result
}

// messageRefKey is the token the submission service appends to its final DATA
// reply, for example "250 2.0.0 accepted; message_ref=<uuid>".
const messageRefKey = "message_ref="

// parseMessageRef extracts the mxRaven message reference from a final DATA
// reply. It returns an empty string when the reply does not carry one.
func parseMessageRef(message string) string {
	index := strings.Index(message, messageRefKey)
	if index < 0 {
		return ""
	}
	value := strings.TrimSpace(message[index+len(messageRefKey):])
	value = strings.TrimPrefix(value, "<")
	if end := strings.IndexAny(value, ">; \t\r\n"); end >= 0 {
		value = value[:end]
	}
	return value
}
