package wabotapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers the branches the behaviour-focused tests leave open: guard
// clauses, error classification edges, and the convenience wrappers.

func TestNewClientWithHTTPClient_panicsOnNilHTTPClient(t *testing.T) {
	assert.PanicsWithValue(t, "httpClient must not be nil", func() {
		NewClientWithHTTPClient("token", "1234567890", nil)
	})
}

// TestClient_SendButtons_and_SendList covers the convenience wrappers end to end.
func TestClient_SendButtons_and_SendList(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.OK"}]}`))
	}))
	defer ts.Close()
	c := newTestClient(ts)

	t.Run("SendButtons", func(t *testing.T) {
		resp, err := c.SendButtons(context.Background(), "16505551234", "pick",
			NewReplyButton("a", "A"))
		require.NoError(t, err)
		assert.Equal(t, "wamid.OK", resp.MessageID())
		assert.Contains(t, string(gotBody), `"type":"button"`)
	})

	t.Run("SendList", func(t *testing.T) {
		resp, err := c.SendList(context.Background(), "16505551234", "pick", "Options",
			ListSection{Title: "S", Rows: []ListRow{{ID: "r1", Title: "Row 1"}}})
		require.NoError(t, err)
		assert.Equal(t, "wamid.OK", resp.MessageID())
		assert.Contains(t, string(gotBody), `"type":"list"`)
	})

	// An invalid config must never reach the wire.
	t.Run("validation still applies", func(t *testing.T) {
		_, err := c.SendButtons(context.Background(), "16505551234", "pick")
		assert.ErrorIs(t, err, ErrNoButtons)
	})
}

func TestSendTemplateConfig_InReplyTo(t *testing.T) {
	cfg := NewSendTemplate("16505551234", "hello_world", "en_US").InReplyTo("wamid.ORIGINAL")
	require.NotNil(t, cfg.Context)
	assert.Equal(t, "wamid.ORIGINAL", cfg.Context.MessageID)
	assert.NoError(t, cfg.Validate())
}

// TestSendTemplateConfig_withComponentReplaces pins that setting body params twice
// replaces the component rather than appending a second one — which would send two
// body components and earn a 132018.
func TestSendTemplateConfig_withComponentReplaces(t *testing.T) {
	cfg := NewSendTemplate("16505551234", "t", "en_US").
		WithBodyParams("first").
		WithBodyParams("second")

	require.Len(t, cfg.Template.Components, 1, "a second call must replace, not append")
	params := cfg.Template.Components[0].Parameters
	require.Len(t, params, 1)
	assert.Equal(t, "second", params[0].Text)
}

// TestBaseMessage_Validate_wrongMessagingProduct covers the guard against a
// hand-built config with the wrong product.
func TestBaseMessage_Validate_wrongMessagingProduct(t *testing.T) {
	cfg := NewSendText("16505551234", "hi")
	cfg.MessagingProduct = MessagingProduct("messenger")

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "messaging_product")
}

func TestBaseMessage_Validate_emptyType(t *testing.T) {
	cfg := NewSendText("16505551234", "hi")
	cfg.Type = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type")
}

func TestSendMessageResponse_MessageID_empty(t *testing.T) {
	var r SendMessageResponse
	assert.Equal(t, "", r.MessageID(), "no messages must yield an empty id, not a panic")
}

// TestSendMessage_undecodableResponse covers the decode-failure branch: a 2xx whose
// body is not a SendMessageResponse.
func TestSendMessage_undecodableResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["not", "an", "object"]`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).SendText(context.Background(), "16505551234", "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestAsAPIError(t *testing.T) {
	assert.Nil(t, AsAPIError(nil))
	assert.Nil(t, AsAPIError(errors.New("plain")))

	apiErr := &APIError{Code: ErrCodeRateLimitHit}
	assert.Same(t, apiErr, AsAPIError(apiErr))
	// Must survive wrapping — the client wraps before returning.
	assert.Same(t, apiErr, AsAPIError(&wrapped{apiErr}))
}

type wrapped struct{ err error }

func (w *wrapped) Error() string { return "wrapped: " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }

// TestAPIError_classifiers walks every classifier over every code, so a code added
// to one bucket and forgotten in another shows up here.
func TestAPIError_classifiers(t *testing.T) {
	for _, tt := range []struct {
		name        string
		err         *APIError
		unreachable bool
		auth        bool
		template    bool
		transient   bool
	}{
		{name: "undeliverable", err: &APIError{Code: ErrCodeMessageUndeliverable}, unreachable: true},
		{name: "not delivered", err: &APIError{Code: ErrCodeNotDelivered}, unreachable: true},
		{name: "business blocked user", err: &APIError{Code: ErrCodeBusinessBlockedUser}, unreachable: true},
		{name: "auth exception", err: &APIError{Code: ErrCodeAuthException}, auth: true},
		{name: "token expired", err: &APIError{Code: ErrCodeAccessTokenExpired}, auth: true},
		{name: "permission denied", err: &APIError{Code: ErrCodePermissionDenied}, auth: true},
		{name: "401 with unknown code", err: &APIError{Code: 999999, HTTPStatusCode: http.StatusUnauthorized}, auth: true},
		{name: "must be template", err: &APIError{Code: ErrCodeMustBeTemplate}, template: true},
		{name: "template validation", err: &APIError{Code: ErrCodeTemplateValidation}, template: true},
		{name: "marketing disabled", err: &APIError{Code: ErrCodeMarketingDisabledOnCloudAPI}, template: true},
		{name: "only marketing templates", err: &APIError{Code: ErrCodeOnlyMarketingTemplates}, template: true},
		{name: "only marketing messages", err: &APIError{Code: ErrCodeOnlyMarketingMessages}, template: true},
		{name: "template unavailable", err: &APIError{Code: ErrCodeTemplateUnavailable}, template: true},
		// Syncing is a template error but NOT transient: it clears in up to 10
		// minutes, far beyond the in-process retry budget.
		{name: "template syncing", err: &APIError{Code: ErrCodeTemplateSyncing}, template: true},
		{name: "unknown error", err: &APIError{Code: ErrCodeUnknown}, transient: true},
		{name: "service unavailable", err: &APIError{Code: ErrCodeServiceUnavailable}, transient: true},
		{name: "http 5xx", err: &APIError{Code: 999999, HTTPStatusCode: http.StatusBadGateway}, transient: true},
		{name: "re-engagement is none of these", err: &APIError{Code: ErrCodeReEngagementRequired}},
		{name: "invalid parameter is none of these", err: &APIError{Code: ErrCodeInvalidParameter}},
		{name: "account restricted is none of these", err: &APIError{Code: ErrCodeAccountRestricted}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.unreachable, tt.err.IsUnreachable(), "IsUnreachable")
			assert.Equal(t, tt.auth, tt.err.IsAuthError(), "IsAuthError")
			assert.Equal(t, tt.template, tt.err.IsTemplateError(), "IsTemplateError")
			assert.Equal(t, tt.transient, tt.err.IsTransient(), "IsTransient")
		})
	}
}

// TestAPIError_Error_formatting covers each optional field in the message, and pins
// that none of them is the request body.
func TestAPIError_Error_formatting(t *testing.T) {
	full := &APIError{
		Code: 131047, ErrorSubcode: 2494055, HTTPStatusCode: 400, Type: "OAuthException",
		Message: "(#131047) Re-engagement message", FBTraceID: "AbCdEf",
		ErrorData: &APIErrorData{Details: "more than 24 hours"},
	}
	s := full.Error()
	for _, want := range []string{
		"code=131047", "subcode=2494055", "http=400", "OAuthException",
		"Re-engagement message", "more than 24 hours", "fbtrace_id=AbCdEf",
	} {
		assert.Contains(t, s, want)
	}

	// Bare error: no optional fields, no panics, no stray separators.
	bare := (&APIError{Code: 1}).Error()
	assert.Contains(t, bare, "code=1")
	assert.NotContains(t, bare, "subcode")
	assert.NotContains(t, bare, "http=")
	assert.NotContains(t, bare, "fbtrace_id")

	// An empty ErrorData must not add empty parentheses.
	assert.NotContains(t, (&APIError{Code: 1, ErrorData: &APIErrorData{}}).Error(), "()")
}

func TestParseRetryAfter(t *testing.T) {
	for _, tt := range []struct {
		header string
		want   int
	}{
		{"", 0},
		{"5", 5},
		{"0", 0},
		{"-3", 0},                            // negative is nonsense
		{"not-a-number", 0},                  // Meta never sends this; be robust anyway
		{"Wed, 21 Oct 2026 07:28:00 GMT", 0}, // the HTTP-date form is not supported
	} {
		resp := &http.Response{Header: http.Header{}}
		if tt.header != "" {
			resp.Header.Set("Retry-After", tt.header)
		}
		assert.Equalf(t, tt.want, parseRetryAfter(resp), "Retry-After: %q", tt.header)
	}
}

// TestRetryDelay covers the backoff ladder and the Retry-After cap.
func TestRetryDelay(t *testing.T) {
	// No hint: exponential from attempt 2.
	assert.Equal(t, 2.0, retryDelay(errors.New("x"), 2).Seconds())
	assert.Equal(t, 4.0, retryDelay(errors.New("x"), 3).Seconds())
	assert.Equal(t, 2.0, retryDelay(nil, 2).Seconds())

	// A present hint wins.
	assert.Equal(t, 7.0, retryDelay(&APIError{RetryAfter: 7}, 2).Seconds())

	// An absurd hint is capped, so a caller cannot be blocked for an hour.
	assert.Equal(t, maxRetryAfter, retryDelay(&APIError{RetryAfter: 99999}, 2))
}

// TestMakeRequest_contextCancelledBeforeSend covers the ctx guard on the first pass.
func TestMakeRequest_contextCancelledBeforeSend(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newTestClient(ts).SendText(ctx, "16505551234", "hi")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestMakeRequest_unreachableHost covers the transport-error branch, and pins that
// the error names the endpoint but never the recipient or body.
func TestMakeRequest_unreachableHost(t *testing.T) {
	c := NewClientWithHTTPClient("tok", "1234567890", &http.Client{})
	c.BaseURL = "http://127.0.0.1:1" // nothing listens here

	_, err := c.SendText(context.Background(), "16505551234", "secret content")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1234567890/messages")
	assert.NotContains(t, err.Error(), "secret content")
	assert.NotContains(t, err.Error(), "tok")
}

// TestMakeRequest_nonJSONErrorBody covers a non-2xx whose body is not a Cloud API
// error envelope — e.g. an HTML page from a proxy.
func TestMakeRequest_nonJSONErrorBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`<html>502 Bad Gateway</html>`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).SendText(context.Background(), "16505551234", "hi")
	require.Error(t, err)

	apiErr := AsAPIError(err)
	require.NotNil(t, apiErr, "a non-envelope failure must still surface as *APIError")
	assert.Equal(t, http.StatusBadGateway, apiErr.HTTPStatusCode)
	// The body may be anything; it must not be echoed.
	assert.NotContains(t, err.Error(), "<html>")
}

// TestMakeRequest_marshalFailure covers the request-encoding branch.
func TestMakeRequest_marshalFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	// A channel cannot be marshalled to JSON.
	_, err := newTestClient(ts).MakeRequest(context.Background(), http.MethodPost, "x/messages", make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}

// TestMakeRequest_givesUpAfterMaxAttempts pins the retry budget.
func TestMakeRequest_givesUpAfterMaxAttempts(t *testing.T) {
	var attempts int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unknown","code":131000}}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	// Retry-After 0 keeps the exponential backoff short enough for a test.
	_, err := c.SendText(context.Background(), "16505551234", "hi")
	require.Error(t, err)
	assert.Equal(t, maxAttempts, attempts)
	assert.Contains(t, err.Error(), "giving up")
}

// TestMakeRequest_getWithNoBody covers the nil-payload path.
func TestMakeRequest_getWithNoBody(t *testing.T) {
	var gotCT string
	var gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotMethod = r.Method
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	raw, err := newTestClient(ts).MakeRequest(context.Background(), http.MethodGet, "x/media", nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(raw))
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Empty(t, gotCT, "a body-less request must not claim a JSON content type")
}

// TestSendListConfig_Validate_rowIDLength covers the row-id cap.
func TestSendListConfig_Validate_rowIDLength(t *testing.T) {
	long := ListSection{Rows: []ListRow{
		{ID: strings.Repeat("x", MaxRowIDLength+1), Title: "T"},
	}}
	err := NewSendList("16505551234", "b", "Btn", long).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max is 200")
}

func TestSendListConfig_Validate_emptyIDsAndTitles(t *testing.T) {
	for _, tt := range []struct {
		name string
		row  ListRow
		want string
	}{
		{"empty row id", ListRow{Title: "T"}, "row id"},
		{"empty row title", ListRow{ID: "r"}, "row title"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := NewSendList("16505551234", "b", "Btn", ListSection{Rows: []ListRow{tt.row}}).Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestSendListConfig_Validate_bodyAndHeaderLength(t *testing.T) {
	section := ListSection{Rows: []ListRow{{ID: "r", Title: "T"}}}

	over := NewSendList("16505551234", strings.Repeat("x", MaxListBodyLength+1), "Btn", section)
	assert.Error(t, over.Validate())

	longBtn := NewSendList("16505551234", "b", strings.Repeat("x", MaxListButtonLength+1), section)
	assert.Error(t, longBtn.Validate())

	longHdr := NewSendList("16505551234", "b", "Btn", section).
		WithHeader(strings.Repeat("x", MaxListHeaderLength+1))
	assert.Error(t, longHdr.Validate())

	empty := NewSendList("16505551234", "", "Btn", section)
	assert.ErrorIs(t, empty.Validate(), ErrEmptyBody)

	noRows := NewSendList("16505551234", "b", "Btn", ListSection{Title: "empty"})
	assert.ErrorIs(t, noRows.Validate(), ErrNoRows)
}

func TestSendButtonsConfig_Validate_bodyAndFooterLength(t *testing.T) {
	btn := NewReplyButton("a", "A")

	over := NewSendButtons("16505551234", strings.Repeat("x", MaxButtonsBodyLength+1), btn)
	assert.Error(t, over.Validate())

	empty := NewSendButtons("16505551234", "", btn)
	assert.ErrorIs(t, empty.Validate(), ErrEmptyBody)

	longFooter := NewSendButtons("16505551234", "b", btn).
		WithFooter(strings.Repeat("x", MaxFooterLength+1))
	assert.Error(t, longFooter.Validate())

	emptyID := NewSendButtons("16505551234", "b", NewReplyButton("", "A"))
	assert.Error(t, emptyID.Validate())

	emptyTitle := NewSendButtons("16505551234", "b", NewReplyButton("a", ""))
	assert.Error(t, emptyTitle.Validate())
}

// TestSendListConfig_Validate_delegatesToBaseMessage covers the base-validation
// branch: a list with no recipient must fail before any list-specific check.
func TestSendListConfig_Validate_delegatesToBaseMessage(t *testing.T) {
	cfg := NewSendList("", "body", "Btn", ListSection{Rows: []ListRow{{ID: "r", Title: "T"}}})
	assert.ErrorIs(t, cfg.Validate(), ErrNoRecipient)
}

// TestMakeRequest_invalidHTTPMethod covers the request-construction failure branch.
func TestMakeRequest_invalidHTTPMethod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	// A method containing a space is not a valid HTTP token.
	_, err := newTestClient(ts).MakeRequest(context.Background(), "BAD METHOD", "x/messages", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
	assert.Contains(t, err.Error(), "x/messages")
}

// errReader fails on the first Read, simulating a connection dropped mid-body.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset mid-body") }

// errBodyTransport returns a 200 whose body cannot be read.
type errBodyTransport struct{}

func (errBodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(errReader{}),
		Header:     http.Header{},
	}, nil
}

// TestMakeRequest_bodyReadFailure covers the response-read failure branch — a
// connection dropped after the status line but before the body arrives.
//
// Worth pinning rather than dismissing as unreachable: the send may well have
// SUCCEEDED at Meta's end, so the caller must see a clear error rather than a
// misleading decode failure, and must not assume the message was not delivered.
func TestMakeRequest_bodyReadFailure(t *testing.T) {
	c := NewClientWithHTTPClient("tok", "1234567890", &http.Client{Transport: errBodyTransport{}})
	c.BaseURL = "http://example.invalid"

	_, err := c.SendText(context.Background(), "16505551234", "secret content")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read response")
	assert.Contains(t, err.Error(), "1234567890/messages")
	assert.NotContains(t, err.Error(), "secret content", "an error must never echo the body")
}
