package admin_test

import (
	"testing"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

func TestPageTokenHelpers(t *testing.T) {
	next := api.OptString{Value: "next", Set: true}
	previous := api.OptString{Value: "previous", Set: true}
	last := api.OptString{Value: "last", Set: true}

	tests := []struct {
		name         string
		res          any
		wantNext     string
		wantPrevious string
		wantLast     string
	}{
		{
			name:     "next only",
			res:      &api.ListListenersOKHeaders{XNextPageToken: next},
			wantNext: "next",
		},
		{
			name:         "cursor navigation",
			res:          &api.ListAuditLogOKHeaders{XNextPageToken: next, XPreviousPageToken: previous, XLastPageToken: last},
			wantNext:     "next",
			wantPrevious: "previous",
			wantLast:     "last",
		},
		{
			name: "unset",
			res:  &api.ListListenersOKHeaders{XNextPageToken: api.OptString{}},
		},
		{
			name: "unsupported response",
			res:  struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := admin.NextPageToken(tt.res); got != tt.wantNext || ok != (tt.wantNext != "") {
				t.Errorf("NextPageToken() = (%q, %v), want (%q, %v)", got, ok, tt.wantNext, tt.wantNext != "")
			}
			if got, ok := admin.PreviousPageToken(tt.res); got != tt.wantPrevious || ok != (tt.wantPrevious != "") {
				t.Errorf("PreviousPageToken() = (%q, %v), want (%q, %v)", got, ok, tt.wantPrevious, tt.wantPrevious != "")
			}
			if got, ok := admin.LastPageToken(tt.res); got != tt.wantLast || ok != (tt.wantLast != "") {
				t.Errorf("LastPageToken() = (%q, %v), want (%q, %v)", got, ok, tt.wantLast, tt.wantLast != "")
			}
		})
	}
}

func TestTraceIDUnset(t *testing.T) {
	if got, ok := admin.TraceID(&api.TenantHeaders{}); ok {
		t.Errorf("TraceID() = (%q, true), want (\"\", false)", got)
	}
	if got, ok := admin.TraceID(struct{}{}); ok {
		t.Errorf("TraceID() = (%q, true), want (\"\", false)", got)
	}
}
