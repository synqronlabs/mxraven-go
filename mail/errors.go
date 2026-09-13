package mail

import (
	"errors"
	"fmt"

	ravenclient "github.com/synqronlabs/raven/client"
)

// SMTPError reports an SMTP reply that rejected a submission.
//
// It is returned when the server refuses a command, for example when the
// sender domain is not authorized or a recipient is rejected. Inspect Code or
// the enhanced status code to make a delivery decision; Message is intended
// for humans and is not stable.
type SMTPError struct {
	// Code is the three-digit SMTP reply code.
	Code int
	// EnhancedCode is the RFC 3463 enhanced status code, when the server
	// supplied one.
	EnhancedCode string
	// Message is the server's human-readable reply text.
	Message string
}

// Error implements the error interface.
func (e *SMTPError) Error() string {
	if e.EnhancedCode != "" {
		return fmt.Sprintf("mail: smtp %d %s: %s", e.Code, e.EnhancedCode, e.Message)
	}
	return fmt.Sprintf("mail: smtp %d: %s", e.Code, e.Message)
}

// Permanent reports whether the failure is permanent (5xx). Retrying the same
// message is unlikely to succeed.
func (e *SMTPError) Permanent() bool {
	return e.Code >= 500 && e.Code < 600
}

// Transient reports whether the failure is transient (4xx). The message may be
// retried later.
func (e *SMTPError) Transient() bool {
	return e.Code >= 400 && e.Code < 500
}

// translateSMTPError converts a raven SMTP error into an [SMTPError] so callers
// do not depend on the raven package. Other errors are returned unchanged.
func translateSMTPError(err error) error {
	if err == nil {
		return nil
	}
	var smtpErr *ravenclient.SMTPError
	if errors.As(err, &smtpErr) {
		return &SMTPError{
			Code:         smtpErr.Code,
			EnhancedCode: smtpErr.EnhancedCode,
			Message:      smtpErr.Message,
		}
	}
	return err
}
