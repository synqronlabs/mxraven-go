package admin

import "github.com/synqronlabs/mxraven-go/admin/api"

// Problem is an RFC 9457 problem details document returned by the API.
//
// It is an alias for the generated [api.Problem].
type Problem = api.Problem

// ProblemFrom extracts the RFC 9457 problem details carried by a generated
// response value.
//
// The generated client returns non-2xx responses as typed response values
// rather than Go errors. ProblemFrom recognises any of those wrappers through
// their shared Problem body and returns a copy. It reports false for success
// responses and for responses that do not carry a problem document.
//
//	res, err := client.GetTenant(ctx, api.GetTenantParams{Slug: "acme"})
//	if err != nil {
//		return err
//	}
//	if problem, ok := admin.ProblemFrom(res); ok {
//		return fmt.Errorf("get tenant: %s: %s", problem.Code, problem.Title)
//	}
func ProblemFrom(res any) (*Problem, bool) {
	type problemResponse interface {
		GetResponse() api.Problem
	}
	r, ok := res.(problemResponse)
	if !ok {
		return nil, false
	}
	problem := r.GetResponse()
	return &problem, true
}

// TraceID returns the X-Trace-ID reported by a generated response value. It
// reports false when the response carries no trace identifier.
func TraceID(res any) (string, bool) {
	type traceResponse interface {
		GetXTraceID() api.OptString
	}
	r, ok := res.(traceResponse)
	if !ok {
		return "", false
	}
	return optionalString(r.GetXTraceID())
}
