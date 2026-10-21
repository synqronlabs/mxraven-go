package admin

import (
	"fmt"
	"reflect"
)

// Do extracts the success value from a generated operation result.
//
// It is the escape hatch for calling the embedded [api.Client] directly while
// still receiving typed errors:
//
//	tenant, err := admin.Do[api.Tenant](client.GetTenant(ctx, params))
//
// A non-2xx response is converted to an [APIError]. Transport, request
// validation, and response decoding failures are returned unchanged. Do
// handles both accessor-based success wrappers and the generated
// status-specific success aliases. T is the response body value type, not a
// pointer. It reports an error when res does not carry a value of type T.
//
// Do is not needed for the tenant-bound services, which already return typed
// values.
func Do[T any](res any, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	if problem, ok := ProblemFrom(res); ok {
		return zero, newAPIError(res, problem)
	}
	if r, ok := res.(responseOf[T]); ok {
		return r.GetResponse(), nil
	}
	if value, ok := responseField[T](res); ok {
		return value, nil
	}
	return zero, fmt.Errorf("admin: unexpected response type %T", res)
}

// Check converts a generated operation result into an error.
//
// It is intended for generated operations that return no response body, such
// as delete operations:
//
//	err := admin.Check(client.DeleteDomain(ctx, params))
//
// A non-2xx response is converted to an *[APIError]. Transport, request
// validation, and response decoding failures are returned unchanged. Check
// returns nil for any successful response, including responses that do carry a
// body.
func Check(res any, err error) error {
	if err != nil {
		return err
	}
	if problem, ok := ProblemFrom(res); ok {
		return newAPIError(res, problem)
	}
	return nil
}

// responseOf is implemented by generated success wrappers that expose their
// body through GetResponse.
type responseOf[T any] interface {
	GetResponse() T
}

// resultPtr extracts a success value and returns a pointer to it.
func resultPtr[T any](res any, err error) (*T, error) {
	value, err := Do[T](res, err)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// responseField handles generated status-specific success aliases that have the
// same fields as their base wrapper but do not inherit its accessor methods.
func responseField[T any](res any) (T, bool) {
	var zero T
	value := reflect.ValueOf(res)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return zero, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return zero, false
	}
	field := value.FieldByName("Response")
	if !field.IsValid() || !field.CanInterface() {
		return zero, false
	}
	body, ok := field.Interface().(T)
	return body, ok
}
