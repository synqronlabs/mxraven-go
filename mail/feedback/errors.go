package feedback

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxErrorBody bounds how much of an error response is read.
const maxErrorBody = 4096

// Error reports a non-success response from the feedback service.
type Error struct {
	// StatusCode is the HTTP response status.
	StatusCode int
	// Message is the service's error message when one was returned, otherwise
	// the HTTP status text.
	Message string
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("feedback: request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("feedback: request failed with status %d: %s", e.StatusCode, e.Message)
}

// Retryable reports whether the request may succeed if retried later. Rate
// limits (429) and server-side failures (5xx) are retryable.
func (e *Error) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// newHTTPError builds an [Error] from a response and consumes a bounded part
// of its body.
func newHTTPError(resp *http.Response) error {
	message := strings.TrimSpace(resp.Status)
	if resp.Body != nil {
		if body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody)); err == nil {
			var payload struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Error) != "" {
				message = strings.TrimSpace(payload.Error)
			}
		}
	}
	return &Error{StatusCode: resp.StatusCode, Message: message}
}
