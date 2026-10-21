package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

const (
	testDomainID   = "22222222-2222-2222-2222-222222222222"
	testListenerID = "33333333-3333-3333-3333-333333333333"
	testTenantID   = "11111111-1111-1111-1111-111111111111"
)

const testDomainJSON = `{
	"id": "` + testDomainID + `",
	"tenant_id": "` + testTenantID + `",
	"domain_name": "example.com",
	"verification_token": "verify-token",
	"sending_enabled": true,
	"dkim_active_selector": "selector1",
	"spf_verified": true,
	"dkim_verified": true,
	"dmarc_verified": true,
	"dmarc_report_address": null,
	"dns_last_checked_at": null,
	"status": "verified",
	"required_customer_records": []
}`

const testListenerJSON = `{
	"id": "` + testListenerID + `",
	"tenant_id": "` + testTenantID + `",
	"display_name": "Outbound",
	"listener_type": "submission",
	"stream_type": "transactional",
	"default_terminal_action_type": "DELIVER",
	"default_terminal_action_payload": {},
	"rspamd_scanning_enabled": false
}`

const testNotificationJSON = `{
	"enabled": true,
	"subject": "mxr.notify.` + testListenerID + `",
	"consumer": "listener-` + testListenerID + `"
}`

const testGrantJSON = `{
	"listener_id": "` + testListenerID + `",
	"listener_display_name": "Outbound",
	"listener_type": "submission",
	"stream_type": "transactional",
	"domain_status": "verified",
	"subdomain_scope": "exact"
}`

func newTestClient(t *testing.T, handler http.Handler) *admin.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := admin.New(admin.WithBaseURL(srv.URL), admin.WithToken("secret"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return client
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func writeProblem(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestWorkspaceTenant(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/tenants/acme"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		writeJSON(w, http.StatusOK, testTenantJSON)
	}))

	ws := client.Workspace(" acme ")
	if got, want := ws.Slug(), "acme"; got != want {
		t.Fatalf("Slug() = %q, want %q", got, want)
	}
	tenant, err := ws.Tenant.Get(context.Background())
	if err != nil {
		t.Fatalf("Tenant.Get() error: %v", err)
	}
	if got, want := string(tenant.Slug), "acme"; got != want {
		t.Errorf("tenant slug = %q, want %q", got, want)
	}
}

func TestDomainsListIteratesPagesLazily(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		query := r.URL.Query()
		if got, want := r.URL.Path, "/v2/tenants/acme/domains"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := query.Get("status"), "verified"; got != want {
			t.Errorf("status = %q, want %q", got, want)
		}
		if got, want := query.Get("page_size"), "2"; got != want {
			t.Errorf("page_size = %q, want %q", got, want)
		}
		if got, want := query.Get("q"), "example"; got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		switch query.Get("page_token") {
		case "":
			w.Header().Set("X-Next-Page-Token", "page-2")
			writeJSON(w, http.StatusOK, "["+testDomainJSON+"]")
		case "page-2":
			writeJSON(w, http.StatusOK, "["+testDomainJSON+"]")
		default:
			t.Errorf("unexpected page_token %q", query.Get("page_token"))
			writeProblem(w, http.StatusBadRequest, testProblemJSON)
		}
	}))

	ws := client.Workspace("acme")
	domains, err := admin.Collect(ws.Domains.List(context.Background(), admin.ListDomainsOptions{
		ListOptions: admin.ListOptions{PageSize: 2},
		Search:      "example",
		Status:      api.DomainStatusVerified,
	}))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got, want := len(domains), 2; got != want {
		t.Errorf("domains = %d, want %d", got, want)
	}
	if got, want := calls.Load(), int32(2); got != want {
		t.Errorf("requests = %d, want %d", got, want)
	}
}

func TestDomainsPages(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next-Page-Token", "next")
		writeJSON(w, http.StatusOK, "["+testDomainJSON+"]")
	}))

	pager := client.Workspace("acme").Domains.Pages(admin.ListDomainsOptions{})
	if !pager.Next(context.Background()) {
		t.Fatalf("Next() = false, want true: %v", pager.Err())
	}
	if got, want := len(pager.Items()), 1; got != want {
		t.Errorf("items = %d, want %d", got, want)
	}
	if got, want := pager.Token(), "next"; got != want {
		t.Errorf("Token() = %q, want %q", got, want)
	}
}

func TestDomainsListStopsEarly(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("X-Next-Page-Token", "page-2")
		writeJSON(w, http.StatusOK, "["+testDomainJSON+"]")
	}))

	ws := client.Workspace("acme")
	domains, err := admin.Take(ws.Domains.List(context.Background(), admin.ListDomainsOptions{}), 1)
	if err != nil {
		t.Fatalf("Take() error: %v", err)
	}
	if got, want := len(domains), 1; got != want {
		t.Errorf("domains = %d, want %d", got, want)
	}
	if got, want := calls.Load(), int32(1); got != want {
		t.Errorf("requests = %d, want %d; iteration must not prefetch", got, want)
	}
}

func TestDomainsListValidatesOptions(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))

	tests := []struct {
		name string
		opts admin.ListDomainsOptions
	}{
		{name: "short search", opts: admin.ListDomainsOptions{Search: "ab"}},
		{name: "page size too small", opts: admin.ListDomainsOptions{ListOptions: admin.ListOptions{PageSize: -1}}},
		{name: "page size too large", opts: admin.ListDomainsOptions{ListOptions: admin.ListOptions{PageSize: admin.MaxPageSize + 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotErr error
			for _, err := range client.Workspace("acme").Domains.List(context.Background(), tt.opts) {
				gotErr = err
				break
			}
			if gotErr == nil {
				t.Fatal("List() error = nil, want validation error")
			}
		})
	}
	if calls.Load() != 0 {
		t.Errorf("requests = %d, want 0; invalid options must not be sent", calls.Load())
	}
}

func TestDomainsListHonorsCanceledContext(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var gotErr error
	for _, err := range client.Workspace("acme").Domains.List(ctx, admin.ListDomainsOptions{}) {
		gotErr = err
		break
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Errorf("List() error = %v, want context.Canceled", gotErr)
	}
	if calls.Load() != 0 {
		t.Errorf("requests = %d, want 0", calls.Load())
	}
}

func TestDomainsListResumesFromToken(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Query().Get("page_token"), "resume"; got != want {
			t.Errorf("page_token = %q, want %q", got, want)
		}
		writeJSON(w, http.StatusOK, "["+testDomainJSON+"]")
	}))

	domains, err := admin.Collect(client.Workspace("acme").Domains.List(context.Background(), admin.ListDomainsOptions{
		ListOptions: admin.ListOptions{PageToken: "resume"},
	}))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got, want := len(domains), 1; got != want {
		t.Errorf("domains = %d, want %d", got, want)
	}
}

func TestDomainsListRepeatedPageToken(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next-Page-Token", "same-token")
		writeJSON(w, http.StatusOK, "["+testDomainJSON+"]")
	}))

	ws := client.Workspace("acme")
	var gotErr error
	for _, err := range ws.Domains.List(context.Background(), admin.ListDomainsOptions{}) {
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Fatal("List() error = nil, want repeated-token error")
	}
}

func TestDomainsGet(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/tenants/acme/domains/"+testDomainID; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		writeJSON(w, http.StatusOK, testDomainJSON)
	}))

	domain, err := client.Workspace("acme").Domains.Get(context.Background(), uuid.MustParse(testDomainID))
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got, want := domain.ID.String(), testDomainID; got != want {
		t.Errorf("domain ID = %q, want %q", got, want)
	}
}

func TestDomainsDelete(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodDelete; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/v2/tenants/acme/domains/"+testDomainID; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.Workspace("acme").Domains.Delete(context.Background(), uuid.MustParse(testDomainID)); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDomainsGetAPIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, http.StatusNotFound, testProblemJSON)
	}))

	_, err := client.Workspace("acme").Domains.Get(context.Background(), uuid.MustParse(testDomainID))
	apiErr, ok := admin.AsAPIError(err)
	if !ok {
		t.Fatalf("AsAPIError(%v) = false, want true", err)
	}
	if got, want := apiErr.Code, "not_found"; got != want {
		t.Errorf("Code = %q, want %q", got, want)
	}
}

func TestDomainsCreateHandlesSuccessStatuses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got, want := r.URL.Path, "/v2/tenants/acme/domains"; got != want {
					t.Errorf("path = %q, want %q", got, want)
				}
				var body api.CreateDomainRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if got, want := body.DomainName, "example.com"; got != want {
					t.Errorf("domain_name = %q, want %q", got, want)
				}
				if body.DmarcReportAddress.Set {
					t.Error("dmarc_report_address set, want omitted")
				}
				writeJSON(w, status, testDomainJSON)
			}))

			domain, err := client.Workspace("acme").Domains.Create(context.Background(), admin.CreateDomainInput{
				DomainName: "example.com",
			})
			if err != nil {
				t.Fatalf("Create() error: %v", err)
			}
			if got, want := domain.ID.String(), testDomainID; got != want {
				t.Errorf("domain ID = %q, want %q", got, want)
			}
		})
	}
}

func TestDomainsCreateDMARCReportAddress(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body api.CreateDomainRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if !body.DmarcReportAddress.Set || body.DmarcReportAddress.Value != "dmarc@example.com" {
			t.Errorf("dmarc_report_address = %+v, want dmarc@example.com", body.DmarcReportAddress)
		}
		writeJSON(w, http.StatusCreated, testDomainJSON)
	}))

	address := "dmarc@example.com"
	if _, err := client.Workspace("acme").Domains.Create(context.Background(), admin.CreateDomainInput{
		DomainName:         "example.com",
		DMARCReportAddress: &address,
	}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
}

func TestDomainsReplaceSendsNullAndValue(t *testing.T) {
	tests := []struct {
		name  string
		value *string
		want  any
	}{
		{name: "clear", value: nil, want: nil},
		{name: "set", value: stringPtr("dmarc@example.com"), want: "dmarc@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got, want := r.Method, http.MethodPut; got != want {
					t.Errorf("method = %q, want %q", got, want)
				}
				if got, want := r.URL.Path, "/v2/tenants/acme/domains/"+testDomainID; got != want {
					t.Errorf("path = %q, want %q", got, want)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				value, ok := body["dmarc_report_address"]
				if !ok {
					t.Fatal("dmarc_report_address missing, want present")
				}
				if value != tt.want {
					t.Errorf("dmarc_report_address = %#v, want %#v", value, tt.want)
				}
				writeJSON(w, http.StatusOK, testDomainJSON)
			}))
			domainID := uuid.MustParse(testDomainID)
			domain, err := client.Workspace("acme").Domains.Replace(context.Background(), domainID, tt.value)
			if err != nil {
				t.Fatalf("Replace() error: %v", err)
			}
			if got, want := domain.ID.String(), testDomainID; got != want {
				t.Errorf("domain ID = %q, want %q", got, want)
			}
		})
	}
}

func TestDomainsListListenerGrants(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v2/tenants/acme/domains/"+testDomainID+"/listener-grants"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		writeJSON(w, http.StatusOK, "["+testGrantJSON+"]")
	}))

	grants, err := admin.Collect(client.Workspace("acme").Domains.ListListenerGrants(
		context.Background(),
		uuid.MustParse(testDomainID),
		admin.ListOptions{},
	))
	if err != nil {
		t.Fatalf("ListListenerGrants() error: %v", err)
	}
	if got, want := len(grants), 1; got != want {
		t.Fatalf("grants = %d, want %d", got, want)
	}
	if got, want := grants[0].ListenerID.String(), testListenerID; got != want {
		t.Errorf("listener ID = %q, want %q", got, want)
	}
}

func TestListenersListFilters(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got, want := query.Get("listener_type"), "mta"; got != want {
			t.Errorf("listener_type = %q, want %q", got, want)
		}
		if got, want := query.Get("rspamd_scanning_enabled"), "true"; got != want {
			t.Errorf("rspamd_scanning_enabled = %q, want %q", got, want)
		}
		writeJSON(w, http.StatusOK, "["+testListenerJSON+"]")
	}))

	enabled := true
	listeners, err := admin.Collect(client.Workspace("acme").Listeners.List(context.Background(), admin.ListListenersOptions{
		ListenerType:          api.ListenerTypeMta,
		RspamdScanningEnabled: &enabled,
	}))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got, want := len(listeners), 1; got != want {
		t.Errorf("listeners = %d, want %d", got, want)
	}
}

func TestListenersMutations(t *testing.T) {
	var created atomic.Bool
	var renamed atomic.Bool
	var actionSet atomic.Bool

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/tenants/acme/listeners":
			var body api.CreateListenerRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if !body.DefaultTerminalActionPayload.Set {
				t.Error("default_terminal_action_payload not set")
			}
			if !body.RspamdScanningEnabled.Set || body.RspamdScanningEnabled.Value {
				t.Errorf("rspamd_scanning_enabled = %+v, want false and set", body.RspamdScanningEnabled)
			}
			created.Store(true)
			writeJSON(w, http.StatusCreated, testListenerJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/tenants/acme/listeners/"+testListenerID:
			writeJSON(w, http.StatusOK, testListenerJSON)
		case r.Method == http.MethodPut && r.URL.Path == "/v2/tenants/acme/listeners/"+testListenerID:
			var body api.UpdateListenerRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if got, want := body.DisplayName, "Renamed"; got != want {
				t.Errorf("display_name = %q, want %q", got, want)
			}
			renamed.Store(true)
			writeJSON(w, http.StatusOK, testListenerJSON)
		case r.Method == http.MethodPut && r.URL.Path == "/v2/tenants/acme/listeners/"+testListenerID+"/rspamd-scanning":
			var body api.UpdateListenerRspamdScanningRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if !body.Enabled {
				t.Error("rspamd enabled = false, want true")
			}
			writeJSON(w, http.StatusOK, testListenerJSON)
		case r.Method == http.MethodPut && r.URL.Path == "/v2/tenants/acme/listeners/"+testListenerID+"/default-terminal-action":
			var body api.UpdateListenerDefaultTerminalActionRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if got, want := body.ActionType, api.DefaultTerminalActionTypeREJECT; got != want {
				t.Errorf("action_type = %q, want %q", got, want)
			}
			if !body.ActionPayload.Set || len(body.ActionPayload.Value) == 0 {
				t.Errorf("action_payload = %+v, want non-empty payload", body.ActionPayload)
			}
			actionSet.Store(true)
			writeJSON(w, http.StatusOK, testListenerJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/tenants/acme/listeners/"+testListenerID:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			writeProblem(w, http.StatusNotFound, testProblemJSON)
		}
	}))

	ctx := context.Background()
	ws := client.Workspace("acme")
	listenerID := uuid.MustParse(testListenerID)

	scanning := false
	if _, err := ws.Listeners.Create(ctx, admin.CreateListenerInput{
		DisplayName:                  "Outbound",
		ListenerType:                 api.ListenerTypeSubmission,
		StreamType:                   api.CreatableListenerStreamTypeTransactional,
		DefaultTerminalActionType:    api.DefaultTerminalActionTypeDELIVER,
		DefaultTerminalActionPayload: admin.DeliverPayload(),
		RspamdScanningEnabled:        &scanning,
	}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if _, err := ws.Listeners.Get(ctx, listenerID); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if _, err := ws.Listeners.Rename(ctx, listenerID, "Renamed"); err != nil {
		t.Fatalf("Rename() error: %v", err)
	}
	if _, err := ws.Listeners.SetRspamdScanning(ctx, listenerID, true); err != nil {
		t.Fatalf("SetRspamdScanning() error: %v", err)
	}
	payload, err := admin.RejectPayload(550, "5.1.1", "No such user")
	if err != nil {
		t.Fatalf("RejectPayload() error: %v", err)
	}
	if _, err := ws.Listeners.SetDefaultTerminalAction(ctx, listenerID, api.DefaultTerminalActionTypeREJECT, payload); err != nil {
		t.Fatalf("SetDefaultTerminalAction() error: %v", err)
	}
	if err := ws.Listeners.Delete(ctx, listenerID); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if !created.Load() || !renamed.Load() || !actionSet.Load() {
		t.Errorf("requests created=%v renamed=%v action=%v, want all true", created.Load(), renamed.Load(), actionSet.Load())
	}
}

func TestListenersNotifications(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := "/v2/tenants/acme/listeners/" + testListenerID + "/notifications"
		if got := r.URL.Path; got != path {
			t.Errorf("path = %q, want %q", got, path)
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, testNotificationJSON)
		case http.MethodPut:
			var body api.UpdateListenerNotificationsRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if !body.Enabled {
				t.Error("enabled = false, want true")
			}
			writeJSON(w, http.StatusOK, testNotificationJSON)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))

	ctx := context.Background()
	ws := client.Workspace("acme")
	listenerID := uuid.MustParse(testListenerID)

	setting, err := ws.Listeners.GetNotifications(ctx, listenerID)
	if err != nil {
		t.Fatalf("GetNotifications() error: %v", err)
	}
	if !setting.Enabled {
		t.Error("notifications enabled = false, want true")
	}
	if _, err := ws.Listeners.SetNotifications(ctx, listenerID, true); err != nil {
		t.Fatalf("SetNotifications() error: %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}
