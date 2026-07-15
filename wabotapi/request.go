package wabotapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/strongo/logus"
)

// Sendable is any config that knows its own Graph API endpoint.
//
// The config struct *is* the JSON request body — it is marshalled directly — so
// a config needs only to declare where it goes. This mirrors the "config struct
// owns its endpoint" idea from bots-api-telegram, without that client's
// url.Values encoding, which was an accommodation to the Telegram Bot API
// specifically. The Cloud API takes JSON bodies.
type Sendable interface {
	// Endpoint returns the path relative to the versioned base, e.g.
	// "1234567890/messages". The phoneNumberID is supplied by the Client.
	Endpoint(phoneNumberID string) string

	// Validate reports whether the config is well-formed, so obviously-bad
	// requests fail locally instead of costing a round trip.
	Validate() error
}

const (
	// maxAttempts bounds the retry loop, including the initial attempt.
	maxAttempts = 3

	// maxRetryAfter caps how long a server-supplied Retry-After will be honored
	// before the client gives up instead of blocking a caller indefinitely.
	maxRetryAfter = 60 * time.Second
)

// Send validates and posts a Sendable, returning the raw JSON result.
//
// The raw payload is returned rather than a decoded type so the envelope is
// handled once, generically, and each call site decodes its own result — the
// one genuinely good pattern in bots-api-telegram's APIResponse.
func (c *Client) Send(ctx context.Context, s Sendable) (json.RawMessage, error) {
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %T: %w", s, err)
	}
	return c.MakeRequest(ctx, http.MethodPost, s.Endpoint(c.phoneNumberID), s)
}

// MakeRequest is the single choke point every API call funnels through.
//
// It marshals reqBody as JSON, applies bearer auth, retries transient failures
// honoring Retry-After, and decodes the Cloud API error envelope. On success it
// returns the raw response body.
func (c *Client) MakeRequest(
	ctx context.Context,
	httpMethod, endpoint string,
	reqBody any,
) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/%s/%s", c.baseURL(), c.graphVersion(), endpoint)

	var payload []byte
	if reqBody != nil {
		var err error
		if payload, err = json.Marshal(reqBody); err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			delay := retryDelay(lastErr, attempt)
			logus.Debugf(ctx, "wabotapi: retrying %s %s in %s (attempt %d/%d)",
				httpMethod, endpoint, delay, attempt, maxAttempts)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled while retrying %s: %w", endpoint, ctx.Err())
			case <-time.After(delay):
			}
		}

		result, err := c.doRequest(ctx, httpMethod, url, endpoint, payload)
		if err == nil {
			return result, nil
		}
		lastErr = err

		apiErr := AsAPIError(err)
		if apiErr == nil || !apiErr.IsTransient() {
			return nil, err
		}
	}
	return nil, fmt.Errorf("giving up on %s after %d attempts: %w", endpoint, maxAttempts, lastErr)
}

// doRequest performs a single attempt.
func (c *Client) doRequest(
	ctx context.Context,
	httpMethod, url, endpoint string,
	payload []byte,
) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	// NewRequestWithContext, not NewRequest: the context must reach the transport
	// so cancellation and deadlines actually apply to the call.
	req, err := http.NewRequestWithContext(ctx, httpMethod, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", endpoint, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// Deliberately reports the endpoint, never the full URL or body: the body
		// carries recipient numbers and message content, and errors reach logs.
		return nil, fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", endpoint, err)
	}

	if apiErr := decodeAPIError(respBody, resp); apiErr != nil {
		logus.Warningf(ctx, "wabotapi: %s %s -> %s", httpMethod, endpoint, apiErr.Error())
		return nil, apiErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-2xx with no parseable error envelope. Surface the status without
		// echoing the body, which may contain message content.
		return nil, &APIError{
			Message:        http.StatusText(resp.StatusCode),
			HTTPStatusCode: resp.StatusCode,
			RetryAfter:     parseRetryAfter(resp),
		}
	}

	return respBody, nil
}

// decodeAPIError returns an *APIError if the body carries a Cloud API error
// envelope, or nil if it does not.
func decodeAPIError(body []byte, resp *http.Response) *APIError {
	var envelope struct {
		Error *APIError `json:"error"`
	}
	// A body that isn't JSON at all is not an API error; let the status check handle it.
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == nil {
		return nil
	}
	envelope.Error.HTTPStatusCode = resp.StatusCode
	envelope.Error.RetryAfter = parseRetryAfter(resp)
	return envelope.Error
}

// parseRetryAfter reads the Retry-After header, supporting the delta-seconds
// form. Returns 0 when absent or unparseable.
func parseRetryAfter(resp *http.Response) int {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return 0
	}
	return secs
}

// retryDelay honors a server-supplied Retry-After, else backs off exponentially.
func retryDelay(lastErr error, attempt int) time.Duration {
	if apiErr := AsAPIError(lastErr); apiErr != nil && apiErr.RetryAfter > 0 {
		d := time.Duration(apiErr.RetryAfter) * time.Second
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	// attempt is >= 2 here, so this yields 2s, 4s, ...
	return time.Duration(1<<(attempt-1)) * time.Second
}
