package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// rawEmailTimeout is the default client timeout used when Fetch is called with
// a nil client.
const rawEmailTimeout = 60 * time.Second

// maxRawEmailBytes bounds a download when the payload does not declare a size.
// The submission service caps accepted messages at 25 MB.
const maxRawEmailBytes = 64 << 20

// Fetch downloads the raw RFC 822 message referenced by a DELIVER_WEBHOOK
// payload.
//
// It sends the payload's short-lived bearer token in the Authorization header
// and verifies the downloaded bytes against the declared size and SHA-256
// digest. A nil client uses a default client with a 60-second timeout.
//
// Fetch is intended for the DELIVER_WEBHOOK "raw_email" object. It is not used
// for NOTIFY_WEBHOOK payloads, which carry no raw message.
func (e RawEmail) Fetch(ctx context.Context, client *http.Client) ([]byte, error) {
	if strings.TrimSpace(e.URL) == "" {
		return nil, errors.New("webhook: raw email URL is empty")
	}
	if strings.TrimSpace(e.AccessToken) == "" {
		return nil, errors.New("webhook: raw email access token is empty")
	}
	if client == nil {
		client = &http.Client{Timeout: rawEmailTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("webhook: build raw email request: %w", err)
	}
	scheme := strings.TrimSpace(e.TokenType)
	if scheme == "" {
		scheme = "Bearer"
	}
	req.Header.Set("Authorization", scheme+" "+e.AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook: fetch raw email: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("webhook: fetch raw email: unexpected status %s", resp.Status)
	}

	limit := int64(maxRawEmailBytes)
	if e.SizeBytes > 0 {
		limit = e.SizeBytes + 1
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("webhook: read raw email: %w", err)
	}
	if e.SizeBytes > 0 && int64(len(body)) != e.SizeBytes {
		return nil, fmt.Errorf("webhook: raw email size = %d bytes, want %d", len(body), e.SizeBytes)
	}
	if digest := strings.TrimSpace(e.SHA256Hex); digest != "" {
		sum := sha256.Sum256(body)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), digest) {
			return nil, errors.New("webhook: raw email SHA-256 mismatch")
		}
	}
	return body, nil
}
