package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

// ProblemError is one actionable validation issue from an RFC 9457 problem
// document. It is an alias for the generated [api.ProblemError].
type ProblemError = api.ProblemError

// APIError is a non-success response returned by the control plane.
//
// The generated client decodes expected error responses as typed values rather
// than Go errors. The tenant-bound services convert those values into an
// *APIError, so callers can use [errors.As] and branch on [APIError.Code]
// instead of inspecting response wrappers.
//
//	domain, err := ws.Domains.Get(ctx, id)
//	if err != nil {
//		var apiErr *admin.APIError
//		if errors.As(err, &apiErr) && apiErr.Code == "not_found" {
//			return errNotFound
//		}
//		return err
//	}
type APIError struct {
	// Status is the HTTP status code.
	Status int
	// Code is the stable machine-readable problem code. Branch on this value,
	// not on the human-readable title or detail.
	Code string
	// Title is the short human-readable summary of the problem category.
	Title string
	// Detail is the safe human-readable detail for this occurrence, when the
	// server provided one.
	Detail string
	// TraceID is the request trace identifier, when available.
	TraceID string
	// RetryAfter is the server-requested delay before retrying, when the
	// response carried a usable Retry-After header.
	RetryAfter time.Duration
	// Errors contains actionable validation issues, when the server provided
	// them.
	Errors []ProblemError
	// Problem is the complete decoded problem document.
	Problem *Problem
}

// Error implements the error interface.
func (e *APIError) Error() string {
	msg := "admin:"
	if e.Status != 0 {
		msg += " " + strconv.Itoa(e.Status)
	}
	if e.Code != "" {
		msg += " " + e.Code
	}
	detail := e.Detail
	if detail == "" {
		detail = e.Title
	}
	if detail != "" {
		msg += ": " + detail
	}
	if msg == "admin:" {
		return "admin: request failed"
	}
	return msg
}

// Retryable reports whether the request may succeed if retried later.
//
// Rate limits (429) and server-side failures (5xx) are retryable.
func (e *APIError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= http.StatusInternalServerError
}

// AsAPIError extracts an *[APIError] from err.
func AsAPIError(err error) (*APIError, bool) {
	var target *APIError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// IsCode reports whether err is an *[APIError] with the given problem code.
func IsCode(err error, code string) bool {
	apiErr, ok := AsAPIError(err)
	return ok && apiErr.Code == code
}

// newAPIError converts a decoded problem response into an *APIError.
func newAPIError(res any, problem *Problem) *APIError {
	e := &APIError{
		Status:  problem.Status,
		Code:    problem.Code,
		Title:   problem.Title,
		Errors:  problem.Errors,
		Problem: problem,
	}
	if detail, ok := optionalString(problem.Detail); ok {
		e.Detail = detail
	}
	if trace, ok := TraceID(res); ok {
		e.TraceID = trace
	}
	if e.TraceID == "" {
		if trace, ok := optionalString(problem.TraceID); ok {
			e.TraceID = trace
		}
	}
	type retryAfterResponse interface {
		GetRetryAfter() api.OptString
	}
	if r, ok := res.(retryAfterResponse); ok {
		if raw, ok := optionalString(r.GetRetryAfter()); ok {
			if delay, ok := parseRetryAfter(raw, time.Now()); ok {
				e.RetryAfter = delay
			}
		}
	}
	return e
}

// problemError returns the *APIError carried by res, or an error describing an
// unexpected response type.
func problemError(res any) error {
	if problem, ok := ProblemFrom(res); ok {
		return newAPIError(res, problem)
	}
	return fmt.Errorf("admin: unexpected response type %T", res)
}
