package wabotapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient returns a Client pointed at ts, exercising the injectable
// BaseURL that makes these tests possible at all.
func newTestClient(ts *httptest.Server) *Client {
	c := NewClientWithHTTPClient("test-token", "1234567890", ts.Client())
	c.BaseURL = ts.URL
	return c
}

func TestNewClient_panicsOnBlankToken(t *testing.T) {
	assert.PanicsWithValue(t, "accessToken must not be empty", func() {
		NewClient("", "1234567890")
	})
	assert.PanicsWithValue(t, "accessToken must not be empty", func() {
		NewClient("   ", "1234567890")
	})
}

func TestNewClient_panicsOnBlankPhoneNumberID(t *testing.T) {
	assert.PanicsWithValue(t, "phoneNumberID must not be empty", func() {
		NewClient("token", "")
	})
}

func TestNewClient_defaults(t *testing.T) {
	c := NewClient("token", "1234567890")
	assert.Equal(t, DefaultBaseURL, c.BaseURL)
	assert.Equal(t, DefaultGraphVersion, c.GraphVersion)
	assert.Equal(t, "1234567890", c.PhoneNumberID())
	require.NotNil(t, c.HTTPClient)
	assert.Equal(t, DefaultTimeout, c.HTTPClient.Timeout,
		"a default client must carry a timeout; an unbounded one can hang a caller forever")
}

// TestClient_zeroValueFieldsFallBack pins that a hand-built Client with empty
// BaseURL/GraphVersion still targets the real API rather than requesting "/".
func TestClient_zeroValueFieldsFallBack(t *testing.T) {
	c := &Client{}
	assert.Equal(t, DefaultBaseURL, c.baseURL())
	assert.Equal(t, DefaultGraphVersion, c.graphVersion())
}

func TestClient_baseURLTrailingSlashTrimmed(t *testing.T) {
	c := NewClient("token", "1234567890")
	c.BaseURL = "https://example.com/"
	assert.Equal(t, "https://example.com", c.baseURL())
}

// TestSendText_requestShape pins the full outbound wire contract: method, path,
// auth header, content type, and body.
func TestSendText_requestShape(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotContentType string
	var gotBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"messaging_product": "whatsapp",
			"contacts": [{"input": "16505551234", "wa_id": "16505551234"}],
			"messages": [{"id": "wamid.TEST123"}]
		}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	resp, err := c.SendText(context.Background(), "16505551234", "hello")
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/"+DefaultGraphVersion+"/1234567890/messages", gotPath)
	assert.Equal(t, "Bearer test-token", gotAuth)
	assert.Equal(t, "application/json", gotContentType)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "16505551234",
		"type": "text",
		"text": {"body": "hello"}
	}`, string(gotBody))

	assert.Equal(t, "wamid.TEST123", resp.MessageID())
	require.Len(t, resp.Contacts, 1)
	assert.Equal(t, "16505551234", resp.Contacts[0].WaID)
}

// TestSendText_graphVersionOverride pins that GraphVersion reaches the URL, so a
// caller can move off the default without a code change.
func TestSendText_graphVersionOverride(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp"}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	c.GraphVersion = "v99.0"
	_, err := c.SendText(context.Background(), "16505551234", "hi")
	require.NoError(t, err)
	assert.Equal(t, "/v99.0/1234567890/messages", gotPath)
}

// TestSendText_reEngagementError pins the 24-hour-window condition, which is the
// case bots-fw currently cannot express: SendMessage has no refusal path.
func TestSendText_reEngagementError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {
			"message": "(#131047) Re-engagement message",
			"type": "OAuthException",
			"code": 131047,
			"error_data": {"messaging_product": "whatsapp", "details": "Message failed to send because more than 24 hours have passed since the customer last replied to this number."},
			"fbtrace_id": "AbCdEf123"
		}}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.SendText(context.Background(), "16505551234", "hello")
	require.Error(t, err)

	apiErr := AsAPIError(err)
	require.NotNil(t, apiErr, "must surface a *APIError")
	assert.True(t, apiErr.IsReEngagementRequired())
	assert.False(t, apiErr.IsTransient(), "re-engagement must not be retried: the window will not reopen on its own")
	assert.False(t, apiErr.IsRateLimited())
	assert.Equal(t, "AbCdEf123", apiErr.FBTraceID)

	// The behavior-interface form callers use without importing this package.
	behaviour, ok := err.(interface{ IsReEngagementRequired() bool })
	require.True(t, ok)
	assert.True(t, behaviour.IsReEngagementRequired())
}

// TestSendText_errorDoesNotLeakRecipientOrBody pins that error text never echoes
// message content or the recipient number, since errors reach logs.
func TestSendText_errorDoesNotLeakRecipientOrBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth access token","type":"OAuthException","code":190}}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.SendText(context.Background(), "16505551234", "secret message content")
	require.Error(t, err)

	assert.NotContains(t, err.Error(), "secret message content")
	assert.NotContains(t, err.Error(), "16505551234")
	assert.NotContains(t, err.Error(), "test-token")
	assert.True(t, AsAPIError(err).IsAuthError())
}

// TestMakeRequest_retriesOnRateLimitHonoringRetryAfter pins that a 429 is retried
// and that Retry-After is honored rather than ignored.
func TestMakeRequest_retriesOnRateLimitHonoringRetryAfter(t *testing.T) {
	var attempts int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limit","code":130429}}`))
			return
		}
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.OK"}]}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	start := time.Now()
	resp, err := c.SendText(context.Background(), "16505551234", "hi")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "a 429 must be retried")
	assert.Equal(t, "wamid.OK", resp.MessageID())
	assert.GreaterOrEqual(t, elapsed, time.Second, "Retry-After: 1 must actually be waited out")
}

// TestMakeRequest_doesNotRetryPermanentErrors pins that a non-transient error
// fails fast instead of burning the retry budget.
func TestMakeRequest_doesNotRetryPermanentErrors(t *testing.T) {
	var attempts int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad param","code":131009}}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.SendText(context.Background(), "16505551234", "hi")
	require.Error(t, err)
	assert.Equal(t, 1, attempts, "a permanent error must not be retried")
}

// TestMakeRequest_contextCancellationStopsRetry pins that ctx actually reaches
// the transport, rather than being held for logging only.
func TestMakeRequest_contextCancellationStopsRetry(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"unavailable","code":131016}}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.SendText(ctx, "16505551234", "hi")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestSend_validatesLocally pins that an invalid config never reaches the wire.
func TestSend_validatesLocally(t *testing.T) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.SendText(context.Background(), "", "hi")
	require.ErrorIs(t, err, ErrNoRecipient)
	assert.False(t, called, "an invalid config must not cost a round trip")
}
