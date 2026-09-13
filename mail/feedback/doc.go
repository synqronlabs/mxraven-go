// Package feedback submits recipient feedback to the mxRaven feedback service.
//
// Tenants can teach the mxRaven spam filter by submitting messages that were
// misclassified. A message is matched to its stored evidence by hash, so the
// exact raw RFC 822 bytes that mxRaven processed must be submitted.
//
//	client, err := feedback.New(
//		feedback.WithBaseURL("https://feedback.mxraven.com"),
//		feedback.WithCredentials(username, secret),
//	)
//	if err != nil {
//		return err
//	}
//
//	result, err := client.LearnSpam(ctx, bytes.NewReader(rawMessage))
//	if err != nil {
//		return err
//	}
//	_ = result
//
// Credentials are the same submission API key used for SMTP submission: the
// username is the key's username and the secret is the key's secret.
//
// The client also exposes the RFC 8058 one-click unsubscribe endpoint that
// recipient mail clients call. Applications that receive feedback programmatically
// normally use [Client.LearnSpam] and [Client.LearnHam]; suppression-list
// management lives in github.com/synqronlabs/mxraven-go/admin.
package feedback
