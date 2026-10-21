package main

import (
	"strings"
	"unicode"
)

var initialisms = map[string]string{
	"api":   "API",
	"ad":    "AD",
	"dmarc": "DMARC",
	"dkim":  "DKIM",
	"http":  "HTTP",
	"id":    "ID",
	"idp":   "IDP",
	"ip":    "IP",
	"json":  "JSON",
	"jwt":   "JWT",
	"ldap":  "LDAP",
	"mta":   "MTA",
	"oauth": "OAuth",
	"saml":  "SAML",
	"smtp":  "SMTP",
	"spf":   "SPF",
	"tls":   "TLS",
	"url":   "URL",
	"uuid":  "UUID",
}

// splitWords splits a CamelCase or PascalCase identifier into words.
func splitWords(value string) []string {
	var words []string
	var current []rune
	runes := []rune(value)
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
			continue
		}
		if len(current) > 0 && unicode.IsUpper(r) {
			previous := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || (unicode.IsUpper(previous) && nextLower) {
				words = append(words, string(current))
				current = nil
			}
		}
		current = append(current, r)
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

// pascal joins words into a PascalCase identifier, normalizing initialisms.
func pascal(words []string) string {
	var builder strings.Builder
	for _, word := range words {
		if word == "" {
			continue
		}
		lower := strings.ToLower(word)
		if initialism, ok := initialisms[lower]; ok {
			builder.WriteString(initialism)
			continue
		}
		runes := []rune(lower)
		runes[0] = unicode.ToUpper(runes[0])
		builder.WriteString(string(runes))
	}
	return builder.String()
}

// pascalFromSnake converts a snake_case name to PascalCase.
func pascalFromSnake(value string) string {
	return pascal(strings.Split(value, "_"))
}

// camelFromSnake converts a snake_case name to lowerCamelCase.
func camelFromSnake(value string) string {
	words := strings.Split(value, "_")
	if len(words) == 0 {
		return ""
	}
	var builder strings.Builder
	first := strings.ToLower(words[0])
	builder.WriteString(first)
	for _, word := range words[1:] {
		if word == "" {
			continue
		}
		lower := strings.ToLower(word)
		if initialism, ok := initialisms[lower]; ok {
			builder.WriteString(initialism)
			continue
		}
		runes := []rune(lower)
		runes[0] = unicode.ToUpper(runes[0])
		builder.WriteString(string(runes))
	}
	return builder.String()
}

// snake converts an identifier to snake_case.
func snake(value string) string {
	words := splitWords(value)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return strings.Join(words, "_")
}

// serviceDef describes one workspace service.
type serviceDef struct {
	Key         string
	Field       string
	Type        string
	Tag         string
	Description string
	Handwritten bool
}

var tagServices = map[string]serviceDef{
	"Tenants":                   {Key: "tenant", Field: "Tenant", Type: "TenantService", Handwritten: true},
	"Domains":                   {Key: "domains", Field: "Domains", Type: "DomainsService", Handwritten: true},
	"Listeners":                 {Key: "listeners", Field: "Listeners", Type: "ListenersService", Handwritten: true},
	"Abuse Control":             {Key: "abuseControl", Field: "AbuseControl", Type: "AbuseControlService"},
	"API Keys":                  {Key: "apiKeys", Field: "APIKeys", Type: "APIKeysService"},
	"Audit Log":                 {Key: "auditLog", Field: "AuditLog", Type: "AuditLogService"},
	"Auto-reply Templates":      {Key: "autoReplyTemplates", Field: "AutoReplyTemplates", Type: "AutoReplyTemplatesService"},
	"Identity Providers":        {Key: "identityProviders", Field: "IdentityProviders", Type: "IdentityProvidersService"},
	"Inbound Routes":            {Key: "inboundRoutes", Field: "InboundRoutes", Type: "InboundRoutesService"},
	"Mail Analytics":            {Key: "mailAnalytics", Field: "MailAnalytics", Type: "MailAnalyticsService"},
	"MTA Rate Limits":           {Key: "mtaRateLimits", Field: "MTARateLimits", Type: "MTARateLimitsService"},
	"Quotas":                    {Key: "quotas", Field: "Quotas", Type: "QuotasService"},
	"Recipient Sets":            {Key: "recipientSets", Field: "RecipientSets", Type: "RecipientSetsService"},
	"Resource Search":           {Key: "resourceSearch", Field: "ResourceSearch", Type: "ResourceSearchService"},
	"Routing Rules":             {Key: "routingRules", Field: "RoutingRules", Type: "RoutingRulesService"},
	"Sending Domain Policies":   {Key: "sendingDomainPolicies", Field: "SendingDomainPolicies", Type: "SendingDomainPoliciesService"},
	"SMTP Forward Destinations": {Key: "smtpForwardDestinations", Field: "SMTPForwardDestinations", Type: "SMTPForwardDestinationsService"},
	"SMTP Relays":               {Key: "smtpRelays", Field: "SMTPRelays", Type: "SMTPRelaysService"},
	"Storage Integrations":      {Key: "storageIntegrations", Field: "StorageIntegrations", Type: "StorageIntegrationsService"},
	"Suppressions":              {Key: "suppressions", Field: "Suppressions", Type: "SuppressionsService"},
	"Webhook Endpoints":         {Key: "webhookEndpoints", Field: "WebhookEndpoints", Type: "WebhookEndpointsService"},
}

// serviceOverrides maps an operation ID to a service when the OpenAPI tag is
// not the best fit.
var serviceOverrides = map[string]string{
	"ListTenantDedicatedIPPools": "DedicatedIPPools",
}

// dedicatedIPPoolsService is a synthetic service for the dedicated IP pool list.
var dedicatedIPPoolsService = serviceDef{
	Key:   "dedicatedIPPools",
	Field: "DedicatedIPPools",
	Type:  "DedicatedIPPoolsService",
	Tag:   "Dedicated IP Pools",
}

// handwrittenOperations maps generated operation names to the hand-written
// facade method that covers them.
var handwrittenOperations = map[string]string{
	"GetTenant":                           "Tenant.Get",
	"ListDomains":                         "Domains.List",
	"CreateDomain":                        "Domains.Create",
	"GetDomain":                           "Domains.Get",
	"ReplaceDomain":                       "Domains.Replace",
	"DeleteDomain":                        "Domains.Delete",
	"ListDomainListenerGrants":            "Domains.ListListenerGrants",
	"ListListeners":                       "Listeners.List",
	"CreateListener":                      "Listeners.Create",
	"GetListener":                         "Listeners.Get",
	"UpdateListener":                      "Listeners.Update",
	"DeleteListener":                      "Listeners.Delete",
	"UpdateListenerDefaultTerminalAction": "Listeners.SetDefaultTerminalAction",
	"UpdateListenerRspamdScanning":        "Listeners.SetRspamdScanning",
	"GetListenerNotifications":            "Listeners.GetNotifications",
	"UpdateListenerNotifications":         "Listeners.SetNotifications",
}

var verbs = map[string]bool{
	"Add":         true,
	"BatchAdd":    true,
	"BatchDelete": true,
	"Claim":       true,
	"Create":      true,
	"Delete":      true,
	"Get":         true,
	"List":        true,
	"Put":         true,
	"Reorder":     true,
	"Replace":     true,
	"Rotate":      true,
	"Search":      true,
	"Send":        true,
	"Set":         true,
	"Update":      true,
}

// methodName derives a concise facade method name from an operation ID.
func methodName(operationID string, service serviceDef) string {
	words := splitWords(operationID)
	verb := ""
	remaining := words
	if len(words) >= 2 {
		two := words[0] + words[1]
		if verbs[two] {
			verb = two
			remaining = words[2:]
		}
	}
	if verb == "" && len(words) > 0 && verbs[words[0]] {
		verb = words[0]
		remaining = words[1:]
	}
	if verb == "" {
		return pascal(words)
	}
	family := familyTokens(service)
	var filtered []string
	for _, word := range remaining {
		if word == "Tenant" {
			continue
		}
		if family[word] {
			continue
		}
		filtered = append(filtered, word)
	}
	if len(filtered) == 0 {
		return verb
	}
	return verb + pascal(filtered)
}

// pageMethodName derives the page-level method name for a list method.
func pageMethodName(method string) string {
	switch method {
	case "List", "Search":
		return "Pages"
	}
	if strings.HasPrefix(method, "List") && len(method) > len("List") {
		rest := strings.TrimPrefix(method, "List")
		return singularize(rest) + "Pages"
	}
	return method + "Pages"
}

// singularize converts a simple plural resource name to singular.
func singularize(value string) string {
	switch {
	case strings.HasSuffix(value, "ies") && len(value) > 3:
		return value[:len(value)-3] + "y"
	case strings.HasSuffix(value, "s") && !strings.HasSuffix(value, "ss"):
		return value[:len(value)-1]
	default:
		return value
	}
}

// familyTokens returns the tokens that identify a service's resource family.
func familyTokens(service serviceDef) map[string]bool {
	tokens := map[string]bool{}
	for _, word := range splitWords(service.Field) {
		tokens[word] = true
	}
	// Add common singular/plural variants.
	field := service.Field
	switch field {
	case "APIKeys":
		tokens["Key"] = true
	case "AutoReplyTemplates":
		tokens["Template"] = true
	case "Domains":
		tokens["Domain"] = true
	case "DedicatedIPPools":
		tokens["Pool"] = true
	case "IdentityProviders":
		tokens["Provider"] = true
	case "InboundRoutes":
		tokens["Route"] = true
	case "Listeners":
		tokens["Listener"] = true
	case "MTARateLimits":
		tokens["Limit"] = true
	case "Quotas":
		tokens["Quota"] = true
	case "RecipientSets":
		tokens["Set"] = true
	case "ResourceSearch":
		tokens["Resources"] = true
	case "RoutingRules":
		tokens["Rule"] = true
	case "SendingDomainPolicies":
		tokens["Policy"] = true
	case "SMTPForwardDestinations":
		tokens["Destination"] = true
	case "SMTPRelays":
		tokens["Relay"] = true
	case "StorageIntegrations":
		tokens["Integration"] = true
	case "Suppressions":
		tokens["Suppression"] = true
	case "WebhookEndpoints":
		tokens["Endpoint"] = true
	}
	return tokens
}
