package wabotapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendCTAURLConfig_Marshal pins the payload against Meta's documented
// interactive cta_url message wire format.
//
// Key verified facts from the reference page:
//   - interactive.type is "cta_url"
//   - action.name is "cta_url"
//   - action.parameters carries display_text and url
//   - body is required; header and footer are optional
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-cta-url-messages
func TestSendCTAURLConfig_Marshal(t *testing.T) {
	cfg := NewSendCTAURL(
		"+16505551234",
		"Check out our latest products!",
		"Visit Us",
		"https://example.com/products",
	).
		WithTextHeader("Special Offer").
		WithFooter("Limited time only")

	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "interactive",
		"interactive": {
			"type": "cta_url",
			"header": {"type": "text", "text": "Special Offer"},
			"body": {"text": "Check out our latest products!"},
			"footer": {"text": "Limited time only"},
			"action": {
				"name": "cta_url",
				"parameters": {
					"display_text": "Visit Us",
					"url": "https://example.com/products"
				}
			}
		}
	}`, string(data))
}

// TestSendCTAURLConfig_MarshalMinimal pins the minimal payload (body + button only).
// Header and footer must be absent from the wire when not set.
func TestSendCTAURLConfig_MarshalMinimal(t *testing.T) {
	cfg := NewSendCTAURL("+16505551234", "See our menu", "View Menu", "https://example.com/menu")

	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "interactive",
		"interactive": {
			"type": "cta_url",
			"body": {"text": "See our menu"},
			"action": {
				"name": "cta_url",
				"parameters": {
					"display_text": "View Menu",
					"url": "https://example.com/menu"
				}
			}
		}
	}`, string(data))
}

// TestSendCTAURLConfig_Validate covers all validation branches.
func TestSendCTAURLConfig_Validate(t *testing.T) {
	valid := func() *SendCTAURLConfig {
		return NewSendCTAURL("+16505551234", "body text", "Open Link", "https://example.com")
	}

	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, valid().Validate())
	})

	t.Run("valid with header and footer", func(t *testing.T) {
		cfg := valid().WithTextHeader("Header").WithFooter("Footer")
		assert.NoError(t, cfg.Validate())
	})

	t.Run("no recipient", func(t *testing.T) {
		cfg := NewSendCTAURL("", "body", "Btn", "https://example.com")
		assert.ErrorIs(t, cfg.Validate(), ErrNoRecipient)
	})

	t.Run("empty body", func(t *testing.T) {
		cfg := NewSendCTAURL("+16505551234", "", "Btn", "https://example.com")
		assert.ErrorIs(t, cfg.Validate(), ErrEmptyBody)
	})

	t.Run("body over limit", func(t *testing.T) {
		cfg := NewSendCTAURL("+16505551234",
			strings.Repeat("x", MaxCtaBodyLength+1), "Btn", "https://example.com")
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 1024")
	})

	t.Run("empty display_text", func(t *testing.T) {
		cfg := NewSendCTAURL("+16505551234", "body", "", "https://example.com")
		assert.ErrorIs(t, cfg.Validate(), ErrCtaDisplayTextEmpty)
	})

	t.Run("display_text at cap passes", func(t *testing.T) {
		cfg := NewSendCTAURL("+16505551234", "body",
			strings.Repeat("x", MaxCtaDisplayTextLength), "https://example.com")
		assert.NoError(t, cfg.Validate())
	})

	t.Run("display_text over limit", func(t *testing.T) {
		cfg := NewSendCTAURL("+16505551234", "body",
			strings.Repeat("x", MaxCtaDisplayTextLength+1), "https://example.com")
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 20")
	})

	t.Run("empty url", func(t *testing.T) {
		cfg := NewSendCTAURL("+16505551234", "body", "Btn", "")
		assert.ErrorIs(t, cfg.Validate(), ErrCtaURLEmpty)
	})

	t.Run("footer over limit", func(t *testing.T) {
		cfg := valid().WithFooter(strings.Repeat("x", MaxCtaFooterLength+1))
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "footer")
	})

	t.Run("text header over limit", func(t *testing.T) {
		cfg := valid().WithTextHeader(strings.Repeat("x", MaxCtaHeaderTextLength+1))
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "header")
	})
}

// TestSendCTAURLConfig_Endpoint pins the message endpoint path.
func TestSendCTAURLConfig_Endpoint(t *testing.T) {
	assert.Equal(t, "1234567890/messages",
		NewSendCTAURL("+16505551234", "b", "Btn", "https://example.com").Endpoint("1234567890"))
}

// TestClient_SendCTAURL_requestShape pins the convenience wrapper end to end.
func TestClient_SendCTAURL_requestShape(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{
			"messaging_product": "whatsapp",
			"contacts": [{"input": "+16505551234", "wa_id": "16505551234"}],
			"messages": [{"id": "wamid.CTA123"}]
		}`))
	}))
	defer ts.Close()

	resp, err := newTestClient(ts).SendCTAURL(
		context.Background(), "+16505551234",
		"Shop our catalogue", "Shop Now", "https://example.com/shop",
	)
	require.NoError(t, err)
	assert.Equal(t, "wamid.CTA123", resp.MessageID())
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "interactive",
		"interactive": {
			"type": "cta_url",
			"body": {"text": "Shop our catalogue"},
			"action": {
				"name": "cta_url",
				"parameters": {
					"display_text": "Shop Now",
					"url": "https://example.com/shop"
				}
			}
		}
	}`, string(gotBody))
}
