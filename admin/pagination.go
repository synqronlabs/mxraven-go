package admin

import "github.com/synqronlabs/mxraven-go/admin/api"

// NextPageToken returns the X-Next-Page-Token reported by a paginated list
// response. It reports false when the response has no next page.
func NextPageToken(res any) (string, bool) {
	type nextPageResponse interface {
		GetXNextPageToken() api.OptString
	}
	r, ok := res.(nextPageResponse)
	if !ok {
		return "", false
	}
	return optionalString(r.GetXNextPageToken())
}

// PreviousPageToken returns the X-Previous-Page-Token reported by a paginated
// list response. It reports false when the response has no previous page.
func PreviousPageToken(res any) (string, bool) {
	type previousPageResponse interface {
		GetXPreviousPageToken() api.OptString
	}
	r, ok := res.(previousPageResponse)
	if !ok {
		return "", false
	}
	return optionalString(r.GetXPreviousPageToken())
}

// LastPageToken returns the X-Last-Page-Token reported by a paginated list
// response. It reports false when the response does not expose a last-page
// token.
func LastPageToken(res any) (string, bool) {
	type lastPageResponse interface {
		GetXLastPageToken() api.OptString
	}
	r, ok := res.(lastPageResponse)
	if !ok {
		return "", false
	}
	return optionalString(r.GetXLastPageToken())
}

func optionalString(v api.OptString) (string, bool) {
	if !v.Set || v.Value == "" {
		return "", false
	}
	return v.Value, true
}
