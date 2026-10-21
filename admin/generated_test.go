package admin_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

const testAPIKeyJSON = `{
	"id": "55555555-5555-5555-5555-555555555555",
	"tenant_id": "` + testTenantID + `",
	"listener_id": "` + testListenerID + `",
	"username": "mxr_tx_55555555"
}`

func TestAPIKeysServiceListAndDelete(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/v2/tenants/acme/listeners/" + testListenerID + "/api-keys"
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base:
			calls.Add(1)
			if got, want := r.URL.Query().Get("page_size"), "2"; got != want {
				t.Errorf("page_size = %q, want %q", got, want)
			}
			switch r.URL.Query().Get("page_token") {
			case "":
				w.Header().Set("X-Next-Page-Token", "keys-2")
				writeJSON(w, http.StatusOK, "["+testAPIKeyJSON+"]")
			case "keys-2":
				writeJSON(w, http.StatusOK, "["+testAPIKeyJSON+"]")
			default:
				t.Errorf("unexpected page_token %q", r.URL.Query().Get("page_token"))
			}
		case r.Method == http.MethodDelete && r.URL.Path == base+"/55555555-5555-5555-5555-555555555555":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			writeProblem(w, http.StatusNotFound, testProblemJSON)
		}
	}))

	ctx := context.Background()
	ws := client.Workspace("acme")
	listenerID := uuid.MustParse(testListenerID)

	keys, err := admin.Collect(ws.APIKeys.List(ctx, listenerID, admin.ListAPIKeysOptions{
		ListOptions: admin.ListOptions{PageSize: 2},
	}))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got, want := len(keys), 2; got != want {
		t.Errorf("keys = %d, want %d", got, want)
	}
	if got, want := calls.Load(), int32(2); got != want {
		t.Errorf("list requests = %d, want %d", got, want)
	}
	if err := ws.APIKeys.Delete(ctx, listenerID, uuid.MustParse("55555555-5555-5555-5555-555555555555")); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestAuditLogListFilters(t *testing.T) {
	createdFrom := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/tenants/acme/audit-log"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		query := r.URL.Query()
		if got, want := query.Get("actor_kind"), "human"; got != want {
			t.Errorf("actor_kind = %q, want %q", got, want)
		}
		if got, want := query.Get("actor_id"), "user-1"; got != want {
			t.Errorf("actor_id = %q, want %q", got, want)
		}
		if got, want := query.Get("status"), "success"; got != want {
			t.Errorf("status = %q, want %q", got, want)
		}
		if got, want := query.Get("q"), "login"; got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		if query.Get("created_from") == "" {
			t.Error("created_from is empty, want a timestamp")
		}
		writeJSON(w, http.StatusOK, "[]")
	}))

	logs, err := admin.Collect(client.Workspace("acme").AuditLog.List(context.Background(), admin.ListAuditLogOptions{
		Search:      "login",
		ActorKind:   api.ListAuditLogActorKindHuman,
		ActorID:     "user-1",
		Status:      api.ListAuditLogStatusSuccess,
		CreatedFrom: &createdFrom,
	}))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("logs = %d, want 0", len(logs))
	}
}

func TestMailAnalyticsQuery(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/tenants/acme/mail-analytics/overview"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		query := r.URL.Query()
		if query.Get("start_at") == "" || query.Get("end_at") == "" {
			t.Errorf("missing analytics interval: %s", r.URL.RawQuery)
		}
		writeProblem(w, http.StatusNotFound, testProblemJSON)
	}))

	_, err := client.Workspace("acme").MailAnalytics.GetOverview(context.Background(), admin.GetMailAnalyticsOverviewQuery{
		StartAt: start,
		EndAt:   end,
	})
	if !admin.IsCode(err, "not_found") {
		t.Fatalf("GetOverview() error = %v, want not_found API error", err)
	}
}

func TestRecipientSetMembersList(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/tenants/acme/recipient-sets/set-1/members"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("q"), "member"; got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		writeJSON(w, http.StatusOK, "[]")
	}))

	members, err := admin.Collect(client.Workspace("acme").RecipientSets.ListMembers(
		context.Background(),
		"set-1",
		admin.ListRecipientSetMembersOptions{Search: "member"},
	))
	if err != nil {
		t.Fatalf("ListMembers() error: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("members = %d, want 0", len(members))
	}
}

func TestResourceSearchRequiresQuery(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))

	var gotErr error
	for _, err := range client.Workspace("acme").ResourceSearch.Search(context.Background(), admin.SearchTenantResourcesOptions{}) {
		gotErr = err
		break
	}
	if gotErr == nil {
		t.Fatal("Search() error = nil, want validation error")
	}
	if calls.Load() != 0 {
		t.Errorf("requests = %d, want 0", calls.Load())
	}
}

func TestAuditLogPages(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Previous-Page-Token", "previous")
		w.Header().Set("X-Next-Page-Token", "next")
		w.Header().Set("X-Last-Page-Token", "last")
		writeJSON(w, http.StatusOK, "[]")
	}))

	pager := client.Workspace("acme").AuditLog.Pages(admin.ListAuditLogOptions{})
	if !pager.Next(context.Background()) {
		t.Fatalf("Next() = false, want true: %v", pager.Err())
	}
	page := pager.Page()
	if page.PreviousToken != "previous" || page.NextToken != "next" || page.LastToken != "last" {
		t.Errorf("Page() = %+v, want previous/next/last tokens", page)
	}
	if pager.PreviousToken() != "previous" || pager.LastToken() != "last" {
		t.Errorf("Pager tokens = %q/%q, want previous/last", pager.PreviousToken(), pager.LastToken())
	}
}

func TestAuditLogPagesValidation(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))

	pager := client.Workspace("acme").AuditLog.Pages(admin.ListAuditLogOptions{
		ListOptions: admin.ListOptions{PageSize: -1},
	})
	if pager.Next(context.Background()) {
		t.Fatal("Next() = true, want false")
	}
	if pager.Err() == nil {
		t.Fatal("Err() = nil, want validation error")
	}
	if calls.Load() != 0 {
		t.Errorf("requests = %d, want 0", calls.Load())
	}
}

func TestIdentityProvidersList(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/tenants/acme/identity-providers"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		writeJSON(w, http.StatusOK, "[]")
	}))

	providers, err := admin.Collect(client.Workspace("acme").IdentityProviders.List(
		context.Background(),
		admin.ListTenantIdentityProvidersOptions{},
	))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("providers = %d, want 0", len(providers))
	}
}
