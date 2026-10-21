package main

import "testing"

func TestMethodName(t *testing.T) {
	tests := []struct {
		operation string
		service   serviceDef
		want      string
	}{
		{operation: "ListAPIKeys", service: tagServices["API Keys"], want: "List"},
		{operation: "CreateAPIKey", service: tagServices["API Keys"], want: "Create"},
		{operation: "ListRecipientSetMembers", service: tagServices["Recipient Sets"], want: "ListMembers"},
		{operation: "BatchAddRecipientSetMembers", service: tagServices["Recipient Sets"], want: "BatchAddMembers"},
		{operation: "GetAutoReplyTemplateContent", service: tagServices["Auto-reply Templates"], want: "GetContent"},
		{operation: "SetAutoReplyTemplateActive", service: tagServices["Auto-reply Templates"], want: "SetActive"},
		{operation: "GetWebhookEndpointByRef", service: tagServices["Webhook Endpoints"], want: "GetByRef"},
		{operation: "RotateWebhookEndpointSecret", service: tagServices["Webhook Endpoints"], want: "RotateSecret"},
		{operation: "GetTenantMTARateLimitOverride", service: tagServices["MTA Rate Limits"], want: "GetOverride"},
		{operation: "GetListenerMTARateLimitOverride", service: tagServices["MTA Rate Limits"], want: "GetListenerOverride"},
		{operation: "GetTenantQuotas", service: tagServices["Quotas"], want: "Get"},
		{operation: "GetTenantIdentityProvisioning", service: tagServices["Identity Providers"], want: "GetProvisioning"},
		{operation: "CreateTenantAzureADIdentityProvider", service: tagServices["Identity Providers"], want: "CreateAzureAD"},
		{operation: "CreateTenantGitHubEnterpriseServerIdentityProvider", service: tagServices["Identity Providers"], want: "CreateGitHubEnterpriseServer"},
		{operation: "CreateTenantSAMLIdentityProvider", service: tagServices["Identity Providers"], want: "CreateSAML"},
		{operation: "SearchTenantResources", service: tagServices["Resource Search"], want: "Search"},
		{operation: "ListTenantDedicatedIPPools", service: dedicatedIPPoolsService, want: "List"},
		{operation: "ListTenantDeliverabilityFindings", service: tagServices["Abuse Control"], want: "ListDeliverabilityFindings"},
		{operation: "ListAuditLog", service: tagServices["Audit Log"], want: "List"},
		{operation: "ListInboundRoutingOutcomes", service: tagServices["Audit Log"], want: "ListInboundRoutingOutcomes"},
		{operation: "GetMailAnalyticsActivityHeatmap", service: tagServices["Mail Analytics"], want: "GetActivityHeatmap"},
	}
	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			if got := methodName(tt.operation, tt.service); got != tt.want {
				t.Errorf("methodName(%q) = %q, want %q", tt.operation, got, tt.want)
			}
		})
	}
}

func TestPageMethodName(t *testing.T) {
	tests := map[string]string{
		"List":                       "Pages",
		"Search":                     "Pages",
		"ListMembers":                "MemberPages",
		"ListDeliveries":             "DeliveryPages",
		"ListListenerGrants":         "ListenerGrantPages",
		"ListInboundRoutingOutcomes": "InboundRoutingOutcomePages",
		"ListDeliverabilityFindings": "DeliverabilityFindingPages",
		"ListDedicatedIPPools":       "DedicatedIPPoolPages",
	}
	for input, want := range tests {
		if got := pageMethodName(input); got != want {
			t.Errorf("pageMethodName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPascalFromSnake(t *testing.T) {
	tests := map[string]string{
		"api_key_id":       "APIKeyID",
		"idp_id":           "IDPID",
		"email_address":    "EmailAddress",
		"created_from":     "CreatedFrom",
		"recipient_domain": "RecipientDomain",
	}
	for input, want := range tests {
		if got := pascalFromSnake(input); got != want {
			t.Errorf("pascalFromSnake(%q) = %q, want %q", input, got, want)
		}
	}
}
