package wabotapi

import (
	"errors"
	"fmt"
	"net/http"
)

// WhatsApp Cloud API error codes.
//
// Classify on Code, never on HTTP status and never on message text:
//
//   - The Cloud API signals throttling with an error code and HTTP 400, NOT with
//     HTTP 429, and it does not send a Retry-After header. Meta's error reference
//     mentions neither. (Several third-party clients, including at least one Go
//     library, wrongly annotate the throttling codes as 429 — do not copy them.)
//   - Meta's docs say code titles "will eventually be deprecated", and the wire
//     text already differs from the documented text for at least 131047, so
//     string-matching Message or details is unsafe.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes
const (
	// ErrCodeAuthException means the app user could not be authenticated,
	// typically an expired or invalidated access token.
	ErrCodeAuthException = 0

	// ErrCodePermissionDenied means permission is not granted or was removed.
	ErrCodePermissionDenied = 10

	// ErrCodeAPITooManyCalls means the app reached its API call rate limit.
	ErrCodeAPITooManyCalls = 4

	// ErrCodeAccessTokenExpired means the access token has expired.
	ErrCodeAccessTokenExpired = 190

	// ErrCodeTemporarilyBlocked means the WhatsApp Business Account is restricted
	// or disabled for violating a platform policy.
	ErrCodeTemporarilyBlocked = 368

	// ErrCodeAccountRestricted is the WABA-restricted variant of 368.
	ErrCodeAccountRestricted = 131031

	// ErrCodeRateLimitIssues means the WhatsApp Business Account hit its rate limit.
	ErrCodeRateLimitIssues = 80007

	// ErrCodeRateLimitHit means Cloud API message throughput has been reached.
	ErrCodeRateLimitHit = 130429

	// ErrCodeSpamRateLimitHit means sending is restricted because previous
	// messages were blocked or flagged for quality.
	ErrCodeSpamRateLimitHit = 131048

	// ErrCodeTemplateCategorizationLimit means the messaging limit was reached due
	// to template categorization violations. It restricts template AND free-form
	// sends, and lifts automatically after the enforcement period.
	ErrCodeTemplateCategorizationLimit = 131064

	// ErrCodeUnknown means the message failed for an unknown reason. Meta's
	// guidance is simply "try again", so it is treated as transient.
	ErrCodeUnknown = 131000

	// ErrCodeServiceUnavailable means a service is temporarily unavailable.
	ErrCodeServiceUnavailable = 131016

	// ErrCodeInvalidParameter means one or more parameter values are invalid.
	ErrCodeInvalidParameter = 131009

	// ErrCodeMessageUndeliverable means the recipient could not be reached: not a
	// WhatsApp number, has not accepted current Terms, or is otherwise unreachable.
	// Closest analogue to Telegram's HTTP 403 "bot was blocked by the user".
	//
	// Note there is no code for the *user* blocking the business — that failure is
	// silent. See ErrCodeBusinessBlockedUser for the inverse.
	ErrCodeMessageUndeliverable = 131026

	// ErrCodeBusinessBlockedUser means this business has blocked the end user via
	// the Block Users API. Unblock before retrying.
	ErrCodeBusinessBlockedUser = 130403

	// ErrCodeReEngagementRequired means more than 24 hours have passed since the
	// recipient last replied to the sender number. Meta's remediation is exact:
	// "Send the recipient a template message instead."
	//
	// This is the WhatsApp customer-service window. It has no Telegram analogue,
	// and bots-fw has no seam through which a platform can refuse a send — see the
	// platform-neutrality gap analysis in the backstage repo.
	ErrCodeReEngagementRequired = 131047

	// ErrCodeNotDelivered means Meta chose not to deliver the message to maintain
	// healthy ecosystem engagement.
	//
	// Retryability is genuinely unclear: Meta says to wait at least 24 hours before
	// resending, and in the same breath that doing so "will only result in another
	// error response". Treated here as non-transient, which satisfies both readings
	// — it only rules out a fast retry.
	ErrCodeNotDelivered = 131049
)

// Template-specific error codes, returned when sending type=template.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes
const (
	// ErrCodeMustBeTemplate means a non-template message was sent where only a
	// template is permitted. The 24h-window sibling of ErrCodeReEngagementRequired.
	ErrCodeMustBeTemplate = 100

	// ErrCodeOnlyMarketingTemplates means only marketing template messages are
	// supported on this endpoint.
	ErrCodeOnlyMarketingTemplates = 131055

	// ErrCodeMarketingDisabledOnCloudAPI means the business set
	// disable_marketing_messages_on_cloud_api=true.
	ErrCodeMarketingDisabledOnCloudAPI = 131063

	// ErrCodeTemplateValidation means there is an issue with the template parameters.
	ErrCodeTemplateValidation = 132018

	// ErrCodeOnlyMarketingMessages means only marketing messages may be sent on
	// this API. Graph API v23.0+.
	ErrCodeOnlyMarketingMessages = 134100

	// ErrCodeTemplateSyncing means a newly created template is still syncing, which
	// can take up to 10 minutes. Transient, but slow to clear. Graph API v23.0+.
	ErrCodeTemplateSyncing = 134101

	// ErrCodeTemplateUnavailable means the template is unavailable for use.
	// Graph API v23.0+.
	ErrCodeTemplateUnavailable = 134102
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

	// Code is the error code. Switch on this, not on Message or HTTPStatusCode.
	Code int `json:"code"`

	// ErrorSubcode is deprecated by Meta and is not returned in v16.0+ responses.
	// Retained only so an older or replayed payload still round-trips.
	//
	// Deprecated: build error handling on Code.
	ErrorSubcode int `json:"error_subcode"`

	// FBTraceID identifies the request in Meta's logs. Include it in bug reports.
	FBTraceID string `json:"fbtrace_id"`

	// ErrorData carries the detail string that usually explains the real cause.
	ErrorData *APIErrorData `json:"error_data,omitempty"`

	// HTTPStatusCode is the transport status. Populated by the client, not by Meta.
	//
	// Informational only. The Cloud API returns HTTP 400 for most failures,
	// including throttling, so it does not discriminate. Classify on Code.
	HTTPStatusCode int `json:"-"`

	// RetryAfter is the Retry-After header value in seconds, when a response
	// carried one. Populated by the client, not by Meta.
	//
	// Meta does not document Retry-After for the Cloud API and is not observed to
	// send it; this is defensive handling for an edge or proxy that might, and is
	// expected to be 0 in practice. Do not treat a 0 as "retry immediately".
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
// Classified on Code alone: the Cloud API returns HTTP 400 for throttling, so the
// status is useless here. The http.StatusTooManyRequests check is a defensive
// fallback for an edge or proxy that throttles ahead of Meta, not for Meta itself.
//
// Exposed as a behavior interface (see the IsForbidden precedent in
// bots-api-telegram) so callers can test it without importing this package:
//
//	if e, ok := err.(interface{ IsRateLimited() bool }); ok && e.IsRateLimited() { ... }
func (e *APIError) IsRateLimited() bool {
	switch e.Code {
	case ErrCodeAPITooManyCalls,
		ErrCodeRateLimitIssues,
		ErrCodeRateLimitHit,
		ErrCodeSpamRateLimitHit,
		ErrCodeTemplateCategorizationLimit:
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

// IsUnreachable reports whether the recipient cannot be reached, making an
// immediate retry pointless. The analogue of Telegram's "bot was blocked by the user".
func (e *APIError) IsUnreachable() bool {
	switch e.Code {
	case ErrCodeMessageUndeliverable, ErrCodeNotDelivered, ErrCodeBusinessBlockedUser:
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

// IsTemplateError reports whether the failure concerns the template itself
// rather than the recipient or the transport.
func (e *APIError) IsTemplateError() bool {
	switch e.Code {
	case ErrCodeMustBeTemplate,
		ErrCodeOnlyMarketingTemplates,
		ErrCodeMarketingDisabledOnCloudAPI,
		ErrCodeTemplateValidation,
		ErrCodeOnlyMarketingMessages,
		ErrCodeTemplateSyncing,
		ErrCodeTemplateUnavailable:
		return true
	}
	return false
}

// IsTransient reports whether retrying the identical request could plausibly
// succeed. Drives the client's own retry loop.
//
// Note ErrCodeTemplateSyncing is deliberately excluded: it clears only after up
// to 10 minutes, far beyond this client's retry budget, so retrying in-process
// would just burn the budget. Callers should re-send later instead.
func (e *APIError) IsTransient() bool {
	if e.IsRateLimited() {
		return true
	}
	switch e.Code {
	case ErrCodeUnknown, ErrCodeServiceUnavailable:
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
