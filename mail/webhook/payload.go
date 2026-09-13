package webhook

// EventType identifies the shape of a decoded webhook payload.
type EventType string

const (
	// EventTypeInboundEmail is a DELIVER_WEBHOOK delivery carrying a complete
	// inbound message.
	EventTypeInboundEmail EventType = "inbound_email"
	// EventTypeStorageStatus is a NOTIFY_WEBHOOK delivery carrying the status
	// of an object-storage write.
	EventTypeStorageStatus EventType = "s3_egress_status"
	// EventTypeDeliveryStatus is a NOTIFY_WEBHOOK delivery carrying the status
	// of an SMTP delivery attempt. The SMTP producer does not send an
	// event_type field, so this value is assigned by [Decode].
	EventTypeDeliveryStatus EventType = "delivery_status"
)

// Event is a decoded webhook delivery. Exactly one of the payload fields is
// non-nil, matching Type.
type Event struct {
	// Type identifies the payload shape.
	Type EventType
	// InboundEmail is set for a DELIVER_WEBHOOK delivery.
	InboundEmail *InboundEmail
	// DeliveryStatus is set for an SMTP NOTIFY_WEBHOOK delivery.
	DeliveryStatus *DeliveryStatus
	// StorageStatus is set for an object-storage NOTIFY_WEBHOOK delivery.
	StorageStatus *StorageStatus
}

// StatusOutcome is the lifecycle state reported by a delivery status webhook.
type StatusOutcome string

const (
	// StatusAttempted means a delivery attempt started.
	StatusAttempted StatusOutcome = "attempted"
	// StatusDelivered means the message was accepted by the destination.
	StatusDelivered StatusOutcome = "delivered"
	// StatusDeferred means the destination temporarily rejected the message.
	StatusDeferred StatusOutcome = "deferred"
	// StatusFailed means delivery failed permanently.
	StatusFailed StatusOutcome = "failed"
	// StatusExpired means the delivery deadline passed before success.
	StatusExpired StatusOutcome = "expired"
	// StatusSuppressed means delivery was skipped by a suppression rule. It is
	// reported for SMTP deliveries only.
	StatusSuppressed StatusOutcome = "suppressed"
)

// TerminalAction is the final routing action recorded for an inbound message.
// Values are the mxRaven worker enum names.
type TerminalAction string

const (
	// TerminalActionDeliverDedicated delivers through a dedicated IP pool.
	TerminalActionDeliverDedicated TerminalAction = "TERMINAL_ACTION_TYPE_DELIVER_DEDICATED"
	// TerminalActionSmarthostRelay relays through the configured smarthost.
	TerminalActionSmarthostRelay TerminalAction = "TERMINAL_ACTION_TYPE_SMARTHOST_RELAY"
	// TerminalActionDeliverWebhook delivers to a webhook endpoint.
	TerminalActionDeliverWebhook TerminalAction = "TERMINAL_ACTION_TYPE_DELIVER_WEBHOOK"
	// TerminalActionSMTPForward forwards the message to an SMTP destination.
	TerminalActionSMTPForward TerminalAction = "TERMINAL_ACTION_TYPE_SMTP_FORWARD"
	// TerminalActionRelay relays the message to another MX.
	TerminalActionRelay TerminalAction = "TERMINAL_ACTION_TYPE_RELAY"
	// TerminalActionS3Store stores the message in object storage.
	TerminalActionS3Store TerminalAction = "TERMINAL_ACTION_TYPE_S3_STORE"
	// TerminalActionAutoReply sends an automatic reply.
	TerminalActionAutoReply TerminalAction = "TERMINAL_ACTION_TYPE_AUTO_REPLY"
	// TerminalActionDrop accepts and discards the message.
	TerminalActionDrop TerminalAction = "TERMINAL_ACTION_TYPE_DROP"
	// TerminalActionReject rejects the message.
	TerminalActionReject TerminalAction = "TERMINAL_ACTION_TYPE_REJECT"
	// TerminalActionDeliver delivers the message normally.
	TerminalActionDeliver TerminalAction = "TERMINAL_ACTION_TYPE_DELIVER"
	// TerminalActionSRSReturn handles a DSN at an SRS return address.
	TerminalActionSRSReturn TerminalAction = "TERMINAL_ACTION_TYPE_SRS_RETURN"
	// TerminalActionLocalDSN generates a local DSN.
	TerminalActionLocalDSN TerminalAction = "TERMINAL_ACTION_TYPE_LOCAL_DSN"
)

// InboundEmail is the DELIVER_WEBHOOK payload.
type InboundEmail struct {
	// EventType is always "inbound_email".
	EventType string `json:"event_type"`
	// TaskID identifies the delivery task and is stable across retries.
	TaskID string `json:"task_id"`
	// TenantID is the owning tenant.
	TenantID string `json:"tenant_id"`
	// ListenerID is the listener that accepted the message.
	ListenerID string `json:"listener_id"`
	// Attempt is the delivery attempt number, starting at 1.
	Attempt uint64 `json:"attempt"`
	// AcceptedAtUTC is the Unix time, in seconds, when mxRaven accepted the
	// message.
	AcceptedAtUTC int64 `json:"accepted_at_utc"`
	// OccurredAtUTC is the Unix time, in seconds, when this delivery was built.
	OccurredAtUTC int64 `json:"occurred_at_utc"`
	// RoutingDecision is the final routing outcome.
	RoutingDecision RoutingDecision `json:"routing_decision"`
	// Verdicts carries spam and malware scan results when scanning ran.
	Verdicts *Verdicts `json:"verdicts,omitempty"`
	// Envelope is the SMTP envelope.
	Envelope Envelope `json:"envelope"`
	// Message summarizes the parsed message headers.
	Message MessageSummary `json:"message"`
	// Headers lists every message header in the order received.
	Headers []HeaderField `json:"headers"`
	// RawEmail grants time-limited access to the raw RFC 822 message.
	RawEmail RawEmail `json:"raw_email"`
}

// RoutingDecision is the final routing outcome for an inbound message.
type RoutingDecision struct {
	// TerminalAction is the final terminal action. Values are the mxRaven
	// worker enum names; see the TerminalAction constants.
	TerminalAction TerminalAction `json:"terminal_action"`
	// MatchedRuleID is the rule that selected the terminal action, when one
	// matched.
	MatchedRuleID string `json:"matched_rule_id,omitempty"`
	// UsedListenerDefault reports whether the listener default was used
	// because no rule produced a terminal action.
	UsedListenerDefault bool `json:"used_listener_default"`
}

// Verdicts carries spam and malware scan results.
type Verdicts struct {
	// Action is the scanner action description.
	Action string `json:"action"`
	// Score is the spam score that was assigned.
	Score float64 `json:"score"`
	// RequiredScore is the threshold the message was compared against.
	RequiredScore float64 `json:"required_score"`
	// IsSpam reports whether the message was classified as spam.
	IsSpam bool `json:"is_spam"`
	// HasMalware reports whether malware was detected.
	HasMalware bool `json:"has_malware"`
	// MalwareNames lists the detected malware signatures.
	MalwareNames []string `json:"malware_names"`
	// IsSkipped reports whether scanning was skipped.
	IsSkipped bool `json:"is_skipped"`
	// Error is the scanner error, when scanning failed.
	Error string `json:"error"`
}

// Envelope is the SMTP envelope of a message.
type Envelope struct {
	// MailFrom is the envelope sender. It may be empty for a null reverse-path.
	MailFrom string `json:"mail_from"`
	// RcptTo lists the envelope recipients.
	RcptTo []string `json:"rcpt_to"`
}

// MessageSummary summarizes the parsed message headers.
type MessageSummary struct {
	// Subject is the decoded Subject header.
	Subject string `json:"subject,omitempty"`
	// From holds the decoded From mailboxes.
	From []string `json:"from,omitempty"`
	// To holds the decoded To mailboxes.
	To []string `json:"to,omitempty"`
	// Cc holds the decoded Cc mailboxes.
	Cc []string `json:"cc,omitempty"`
	// MessageID is the Message-ID header.
	MessageID string `json:"message_id,omitempty"`
	// Date is the raw Date header.
	Date string `json:"date,omitempty"`
}

// HeaderField is one message header occurrence.
type HeaderField struct {
	// Name is the header field name.
	Name string `json:"name"`
	// Value is the header field value.
	Value string `json:"value"`
}

// RawEmail grants time-limited access to the raw RFC 822 message. Use
// [RawEmail.Fetch] to download the content.
type RawEmail struct {
	// URL is the message download URL.
	URL string `json:"url"`
	// TokenType is the authorization scheme, normally "Bearer".
	TokenType string `json:"token_type"`
	// AccessToken is the bearer token for the download URL. Treat it as a
	// secret and do not log it.
	AccessToken string `json:"access_token"`
	// ExpiresAtUTC is the Unix time, in seconds, when the token expires.
	ExpiresAtUTC int64 `json:"expires_at_utc"`
	// SizeBytes is the raw message size.
	SizeBytes int64 `json:"size_bytes"`
	// SHA256Hex is the lowercase hex SHA-256 of the raw message bytes.
	SHA256Hex string `json:"sha256_hex"`
	// ContentType is the parsed message media type.
	ContentType string `json:"content_type,omitempty"`
}

// DeliveryStatus is the SMTP NOTIFY_WEBHOOK payload.
type DeliveryStatus struct {
	// TaskID identifies the delivery task and is stable across attempts.
	TaskID string `json:"task_id"`
	// TenantID is the owning tenant.
	TenantID string `json:"tenant_id"`
	// ListenerID is the source listener.
	ListenerID string `json:"listener_id"`
	// Status is the delivery outcome.
	Status StatusOutcome `json:"status"`
	// Attempt is the delivery attempt number, starting at 1.
	Attempt uint64 `json:"attempt"`
	// AcceptedAtUTC is the Unix time, in seconds, when mxRaven accepted the
	// message.
	AcceptedAtUTC int64 `json:"accepted_at_utc"`
	// OccurredAtUTC is the Unix time, in seconds, when this status was built.
	OccurredAtUTC int64 `json:"occurred_at_utc"`
	// SourceIP is the egress source address, when known.
	SourceIP string `json:"source_ip,omitempty"`
	// DestinationDomain is the recipient domain.
	DestinationDomain string `json:"destination_domain,omitempty"`
	// RemoteHost is the remote MTA hostname, when known.
	RemoteHost string `json:"remote_host,omitempty"`
	// SMTPCode is the remote SMTP reply code.
	SMTPCode int `json:"smtp_code,omitempty"`
	// EnhancedStatusCode is the RFC 3463 enhanced status code.
	EnhancedStatusCode string `json:"enhanced_status_code,omitempty"`
	// RemoteResponse is the remote reply text, truncated to 512 bytes.
	RemoteResponse string `json:"remote_response,omitempty"`
	// NextRetryAtUTC is the Unix time, in seconds, of the next attempt for a
	// deferred status.
	NextRetryAtUTC int64 `json:"next_retry_at_utc,omitempty"`
	// CorrelationTaskID links a generated DSN back to its original task.
	CorrelationTaskID string `json:"correlation_task_id,omitempty"`
}

// StorageStatus is the object-storage NOTIFY_WEBHOOK payload.
type StorageStatus struct {
	// EventType is always "s3_egress_status".
	EventType string `json:"event_type"`
	// TaskID identifies the storage task.
	TaskID string `json:"task_id,omitempty"`
	// TenantID is the owning tenant.
	TenantID string `json:"tenant_id,omitempty"`
	// ListenerID is the source listener.
	ListenerID string `json:"listener_id,omitempty"`
	// Status is the storage outcome.
	Status StatusOutcome `json:"status"`
	// Attempt is the delivery attempt number, starting at 1.
	Attempt uint64 `json:"attempt"`
	// AcceptedAtUTC is the Unix time, in seconds, when mxRaven accepted the
	// message.
	AcceptedAtUTC int64 `json:"accepted_at_utc,omitempty"`
	// OccurredAtUTC is the Unix time, in seconds, when this status was built.
	OccurredAtUTC int64 `json:"occurred_at_utc"`
	// StorageRef identifies the storage integration.
	StorageRef string `json:"storage_ref,omitempty"`
	// BucketName is the destination bucket.
	BucketName string `json:"bucket_name,omitempty"`
	// ObjectKey is the stored object key.
	ObjectKey string `json:"object_key,omitempty"`
	// EndpointHost is the storage endpoint host.
	EndpointHost string `json:"endpoint_host,omitempty"`
	// StatusCode is the storage provider response status.
	StatusCode int `json:"status_code,omitempty"`
	// ErrorCode is the storage provider error code.
	ErrorCode string `json:"error_code,omitempty"`
	// Message describes the failure, truncated to 512 bytes.
	Message string `json:"message,omitempty"`
	// NextRetryAtUTC is the Unix time, in seconds, of the next attempt for a
	// deferred status.
	NextRetryAtUTC int64 `json:"next_retry_at_utc,omitempty"`
}
