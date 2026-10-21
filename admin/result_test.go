package admin_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

func TestDoUnwrapsAccessorResponse(t *testing.T) {
	want := api.Domain{ID: uuid.MustParse(testDomainID)}
	got, err := admin.Do[api.Domain](&api.GetDomainOKHeaders{Response: want}, nil)
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("Do() = %v, want %v", got, want)
	}
}

func TestDoUnwrapsStatusAliasResponse(t *testing.T) {
	want := api.Domain{ID: uuid.MustParse(testDomainID)}
	got, err := admin.Do[api.Domain](&api.CreateDomainCreated{Response: want}, nil)
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("Do() = %v, want %v", got, want)
	}
}

func TestDoUnwrapsSliceResponse(t *testing.T) {
	want := []api.Domain{{ID: uuid.MustParse(testDomainID)}}
	got, err := admin.Do[[]api.Domain](&api.ListDomainsOKHeaders{Response: want}, nil)
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	if len(got) != 1 || got[0].ID != want[0].ID {
		t.Errorf("Do() = %v, want %v", got, want)
	}
}

func TestDoConvertsProblemToAPIError(t *testing.T) {
	res := &api.NotFoundHeaders{
		XTraceID: api.NewOptString("header-trace"),
		Response: api.Problem{
			Type:    "urn:mxraven:problem:not_found",
			Title:   "Not Found",
			Status:  404,
			Code:    "not_found",
			Detail:  api.NewOptString("domain not found"),
			TraceID: api.NewOptString("problem-trace"),
			Errors: []api.ProblemError{
				{Code: "missing", Detail: "required value"},
			},
		},
	}

	_, err := admin.Do[api.Domain](res, nil)
	if err == nil {
		t.Fatal("Do() error = nil, want API error")
	}
	apiErr, ok := admin.AsAPIError(err)
	if !ok {
		t.Fatalf("AsAPIError(%v) = false, want true", err)
	}
	if got, want := apiErr.Status, 404; got != want {
		t.Errorf("Status = %d, want %d", got, want)
	}
	if got, want := apiErr.Code, "not_found"; got != want {
		t.Errorf("Code = %q, want %q", got, want)
	}
	if got, want := apiErr.Detail, "domain not found"; got != want {
		t.Errorf("Detail = %q, want %q", got, want)
	}
	if got, want := apiErr.TraceID, "header-trace"; got != want {
		t.Errorf("TraceID = %q, want %q", got, want)
	}
	if got, want := len(apiErr.Errors), 1; got != want {
		t.Errorf("Errors = %d, want %d", got, want)
	}
	if apiErr.Problem == nil || apiErr.Problem.Code != "not_found" {
		t.Errorf("Problem = %+v, want decoded problem", apiErr.Problem)
	}
	if got, want := apiErr.Error(), "admin: 404 not_found: domain not found"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !admin.IsCode(err, "not_found") {
		t.Error("IsCode(error, not_found) = false, want true")
	}
	if admin.IsCode(err, "conflict") {
		t.Error("IsCode(error, conflict) = true, want false")
	}
	if apiErr.Retryable() {
		t.Error("Retryable() = true, want false")
	}
}

func TestDoUsesProblemTraceIDFallback(t *testing.T) {
	res := &api.NotFoundHeaders{
		Response: api.Problem{
			Status:  404,
			Code:    "not_found",
			TraceID: api.NewOptString("problem-trace"),
		},
	}
	_, err := admin.Do[api.Domain](res, nil)
	apiErr, ok := admin.AsAPIError(err)
	if !ok {
		t.Fatalf("AsAPIError(%v) = false, want true", err)
	}
	if got, want := apiErr.TraceID, "problem-trace"; got != want {
		t.Errorf("TraceID = %q, want %q", got, want)
	}
}

func TestDoParsesRetryAfter(t *testing.T) {
	res := &api.TooManyRequestsHeaders{
		RetryAfter: api.NewOptString("3"),
		Response: api.Problem{
			Status: 429,
			Code:   "rate_limited",
		},
	}
	_, err := admin.Do[api.Domain](res, nil)
	apiErr, ok := admin.AsAPIError(err)
	if !ok {
		t.Fatalf("AsAPIError(%v) = false, want true", err)
	}
	if got, want := apiErr.RetryAfter, 3*time.Second; got != want {
		t.Errorf("RetryAfter = %s, want %s", got, want)
	}
	if !apiErr.Retryable() {
		t.Error("Retryable() = false, want true")
	}
}

func TestDoPreservesTransportErrors(t *testing.T) {
	wantErr := errors.New("connection failed")
	_, err := admin.Do[api.Domain](nil, wantErr)
	if !errors.Is(err, wantErr) {
		t.Errorf("Do() error = %v, want %v", err, wantErr)
	}
}

func TestDoRejectsUnexpectedResponse(t *testing.T) {
	if _, err := admin.Do[api.Domain](struct{}{}, nil); err == nil {
		t.Fatal("Do() error = nil, want unexpected response error")
	}
}

func TestCheck(t *testing.T) {
	if err := admin.Check(&api.DeleteDomainNoContent{}, nil); err != nil {
		t.Errorf("Check(success) = %v, want nil", err)
	}

	wantErr := errors.New("connection failed")
	if err := admin.Check(nil, wantErr); !errors.Is(err, wantErr) {
		t.Errorf("Check(transport error) = %v, want %v", err, wantErr)
	}

	res := &api.NotFoundHeaders{Response: api.Problem{Status: 404, Code: "not_found"}}
	err := admin.Check(res, nil)
	if !admin.IsCode(err, "not_found") {
		t.Errorf("Check(problem) = %v, want not_found API error", err)
	}
}
