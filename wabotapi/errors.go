package wabotapi

import (
	"errors"
	"fmt"
	"net/http"
)

// WhatsApp Cloud API error codes.
//
// The Cloud API returns a numeric code (and sometimes an error_subcode) rather
// than distinct HTTP statuses, so callers must switch on these to distinguish
// "retry later" from "never retry this".
//
// https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes
const (
	// ErrCodeAuthException means the request could not be authenticated.
	ErrCodeAuthException = 0

	// ErrCodeAPITooManyCalls is the Graph-level throttle.
	ErrCodeAPITooManyCalls = 4

	// ErrCodePermissionDenied means the app lacks permission for this call.
	ErrCodePermissionDenied = 10

	// ErrCodeAccessTokenExpired means the token must be refreshed.
	ErrCodeAccessTokenExpired = 190

	// ErrCodeTemporarilyBlocked means the account is blocked for policy violations.
	ErrCodeTemporarilyBlocked = 368

	// ErrCodeRateLimitIssues is a generic rate-limit signal.
	ErrCodeRateLimitIssues = 80007

	// ErrCodeRateLimitHit is the Cloud API message throughput limit.
	ErrCodeRateLimitHit = 130429

	// ErrCodeMessageUndeliverable means the recipient could not be reached.
	// Closest analogue to Telegram's HTTP 403 "bot was blocked by the user".
	ErrCodeMessageUndeliverable = 131026

	// ErrCodeReEngagementRequired means more than 24 hours have passed since the
	// recipient last replied, so only a pre-approved template may be sent.
	//
	// This is the WhatsApp customer-service window. It has no Telegram analogue,
	// and bots-fw currently has no seam through which a platform can refuse a
	// send — see the platform-neutrality gap analysis in the backstage repo.
	ErrCodeReEngagementRequired = 131047

	// ErrCodeSpamRateLimitHit means the number has been flagged for spam.
	ErrCodeSpamRateLimitHit = 131048

	// ErrCodeNotDelivered means Meta chose not to deliver the message.
	ErrCodeNotDelivered = 131049
)

// Sentinel errors for conditions detected client-side, before any request.
var (
	// ErrNoRecipient is returned when a message config has a blank To.
	ErrNoRecipient = errors.New("recipient (To) must not be empty")

	// ErrEmptyBody is returned when a message carries no content.
	ErrEmptyBody = errors.New("message body must not be empty")
)

// APIError is the WhatsApp Cloud API error payload.
//
// Unlike Telegram's APIResponse, the Cloud API has no "ok" envelope: a
// successful call returns the result object directly, and a failed one returns
// {"error": {...}}.
//
// https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes
type APIError struct {
	// Message is Meta's human-readable summary. Safe to log.
	Message string `json:"message"`

	// Type is the exception class, e.g. "OAuthException".
	Type string `json:"type"`

	// Code is the error code. Switch on this, not on Message.
	Code int `json:"code"`

	// ErrorSubcode further qualifies Code. Zero when absent.
	ErrorSubcode int `json:"error_subcode"`

	// FBTraceID identifies the request in Meta's logs. Include it in bug reports.
	FBTraceID string `json:"fbtrace_id"`

	// ErrorData carries the detail string that usually explains the real cause.
	ErrorData *APIErrorData `json:"error_data,omitempty"`

	// HTTPStatusCode is the transport status. Populated by the client, not by Meta.
	HTTPStatusCode int `json:"-"`

	// RetryAfter is the Retry-After header value, when the response carried one.
	// Zero means the server gave no hint. Populated by the client, not by Meta.
	RetryAfter int `json:"-"`
}

// APIErrorData is the nested detail object on an APIError.
type APIErrorData struct {
	MessagingProduct string `json:"messaging_product"`

	// Details is usually the most specific description of what went wrong.
	Details string `json:"details"`
}

// Error implements error.
//
// Deliberately never includes the request URL or body: the URL carries no token
// (WhatsApp authenticates via header, unlike Telegram's in-path token), but the
// body carries recipient phone numbers and message content, and errors reach logs.
func (e *APIError) Error() string {
	s := fmt.Sprintf("whatsapp api error: code=%d", e.Code)
	if e.ErrorSubcode != 0 {
		s += fmt.Sprintf(" subcode=%d", e.ErrorSubcode)
	}
	if e.HTTPStatusCode != 0 {
		s += fmt.Sprintf(" http=%d", e.HTTPStatusCode)
	}
	if e.Type != "" {
		s += fmt.Sprintf(" type=%s", e.Type)
	}
	if e.Message != "" {
		s += fmt.Sprintf(": %s", e.Message)
	}
	if e.ErrorData != nil && e.ErrorData.Details != "" {
		s += fmt.Sprintf(" (%s)", e.ErrorData.Details)
	}
	if e.FBTraceID != "" {
		s += fmt.Sprintf(" [fbtrace_id=%s]", e.FBTraceID)
	}
	return s
}

// IsRateLimited reports whether the request was throttled and may be retried later.
//
// Exposed as a behavior interface (see the IsForbidden precedent in
// bots-api-telegram) so callers can test it without importing this package:
//
//	if e, ok := err.(interface{ IsRateLimited() bool }); ok && e.IsRateLimited() { ... }
func (e *APIError) IsRateLimited() bool {
	switch e.Code {
	case ErrCodeAPITooManyCalls, ErrCodeRateLimitIssues, ErrCodeRateLimitHit, ErrCodeSpamRateLimitHit:
		return true
	}
	return e.HTTPStatusCode == http.StatusTooManyRequests
}

// IsReEngagementRequired reports whether the send failed because the 24-hour
// customer-service window has closed and only a pre-approved template may be sent.
//
// This is the condition bots-fw cannot currently express: WebhookResponder.SendMessage
// has no "not permitted right now" return path.
func (e *APIError) IsReEngagementRequired() bool {
	return e.Code == ErrCodeReEngagementRequired
}

// IsUnreachable reports whether the recipient cannot be reached at all, making
// a retry pointless. The analogue of Telegram's "bot was blocked by the user".
func (e *APIError) IsUnreachable() bool {
	switch e.Code {
	case ErrCodeMessageUndeliverable, ErrCodeNotDelivered:
		return true
	}
	return false
}

// IsAuthError reports whether the access token is missing, invalid, or expired.
func (e *APIError) IsAuthError() bool {
	switch e.Code {
	case ErrCodeAuthException, ErrCodeAccessTokenExpired, ErrCodePermissionDenied:
		return true
	}
	return e.HTTPStatusCode == http.StatusUnauthorized
}

// IsTransient reports whether retrying the identical request could plausibly
// succeed. Drives the client's own retry loop.
func (e *APIError) IsTransient() bool {
	if e.IsRateLimited() {
		return true
	}
	return e.HTTPStatusCode >= 500
}

// AsAPIError extracts an *APIError from an error chain, or returns nil.
func AsAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}
