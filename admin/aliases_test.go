package admin_test

import (
	"testing"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

func acceptAdminDomain(admin.Domain)                               {}
func acceptAPIDomain(api.Domain)                                   {}
func acceptAdminTenant(admin.Tenant)                               {}
func acceptAdminListener(admin.Listener)                           {}
func acceptAdminAPIKey(admin.APIKey)                               {}
func acceptAdminAuditLog(admin.AuditLog)                           {}
func acceptAdminMailAnalyticsOverview(admin.MailAnalyticsOverview) {}
func acceptAdminActionPayload(admin.DefaultTerminalActionPayload)  {}

func TestGeneratedAliases(t *testing.T) {
	acceptAPIDomain(admin.Domain{})
	acceptAdminDomain(api.Domain{})
	acceptAdminTenant(api.Tenant{})
	acceptAdminListener(api.Listener{})
	acceptAdminAPIKey(api.APIKey{})
	acceptAdminAuditLog(api.AuditLog{})
	acceptAdminMailAnalyticsOverview(api.MailAnalyticsOverview{})
	acceptAdminActionPayload(api.DefaultTerminalActionPayload{})

	if admin.DomainStatusVerified != api.DomainStatusVerified {
		t.Errorf("DomainStatusVerified = %q, want %q", admin.DomainStatusVerified, api.DomainStatusVerified)
	}
	if admin.ListenerTypeSubmission != api.ListenerTypeSubmission {
		t.Errorf("ListenerTypeSubmission = %q, want %q", admin.ListenerTypeSubmission, api.ListenerTypeSubmission)
	}
	if admin.DefaultTerminalActionTypeDELIVER != api.DefaultTerminalActionTypeDELIVER {
		t.Errorf("DefaultTerminalActionTypeDELIVER = %q, want %q", admin.DefaultTerminalActionTypeDELIVER, api.DefaultTerminalActionTypeDELIVER)
	}
	if admin.SuppressionReasonUnsubscribe != api.SuppressionReasonUnsubscribe {
		t.Errorf("SuppressionReasonUnsubscribe = %q, want %q", admin.SuppressionReasonUnsubscribe, api.SuppressionReasonUnsubscribe)
	}
}
