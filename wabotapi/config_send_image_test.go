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

// TestSendImageByIDConfig_Marshal pins the payload against Meta's documented
// image-by-ID example from the image-messages reference page.
//
// Meta recommends id over link: "We recommend using id and an uploaded media
// asset ID instead" for better performance.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/image-messages
func TestSendImageByIDConfig_Marshal(t *testing.T) {
	cfg := NewSendImageByID("+16505551234", "1479537139650973").
		WithCaption("The best succulent ever?")

	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "image",
		"image": {
			"id": "1479537139650973",
			"caption": "The best succulent ever?"
		}
	}`, string(data))
}

// TestSendImageByLinkConfig_Marshal pins the payload for the link variant.
// No caption in this case; the field must be omitted from the wire payload.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/image-messages
func TestSendImageByLinkConfig_Marshal(t *testing.T) {
	cfg := NewSendImageByLink("+16505551234", "https://example.com/photo.jpg")

	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "image",
		"image": {
			"link": "https://example.com/photo.jpg"
		}
	}`, string(data))
}

// TestSendImageConfig_Validate covers all validation branches.
func TestSendImageConfig_Validate(t *testing.T) {
	t.Run("valid by id", func(t *testing.T) {
		assert.NoError(t, NewSendImageByID("+16505551234", "media123").Validate())
	})

	t.Run("valid by link", func(t *testing.T) {
		assert.NoError(t, NewSendImageByLink("+16505551234", "https://example.com/img.png").Validate())
	})

	t.Run("valid by id with caption at cap", func(t *testing.T) {
		cfg := NewSendImageByID("+16505551234", "media123").
			WithCaption(strings.Repeat("x", MaxImageCaptionLength))
		assert.NoError(t, cfg.Validate())
	})

	t.Run("no recipient", func(t *testing.T) {
		assert.ErrorIs(t, NewSendImageByID("", "media123").Validate(), ErrNoRecipient)
	})

	t.Run("no source", func(t *testing.T) {
		// A hand-built config with both blank — cannot happen via constructors but
		// still possible with a zero-value field override.
		cfg := &SendImageConfig{
			BaseMessage: newBaseMessage("+16505551234", MessageTypeImage),
		}
		assert.ErrorIs(t, cfg.Validate(), ErrImageNoSource)
	})

	t.Run("both id and link", func(t *testing.T) {
		cfg := &SendImageConfig{
			BaseMessage: newBaseMessage("+16505551234", MessageTypeImage),
			Image: ImageObject{
				ID:   "media123",
				Link: "https://example.com/img.jpg",
			},
		}
		assert.ErrorIs(t, cfg.Validate(), ErrImageBothSources)
	})

	t.Run("caption over limit", func(t *testing.T) {
		cfg := NewSendImageByID("+16505551234", "media123").
			WithCaption(strings.Repeat("x", MaxImageCaptionLength+1))
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 1024")
	})
}

// TestSendImageConfig_Endpoint pins that image messages POST to the messages path.
func TestSendImageConfig_Endpoint(t *testing.T) {
	assert.Equal(t, "1234567890/messages",
		NewSendImageByID("+16505551234", "media123").Endpoint("1234567890"))
}

// TestSendImageConfig_InReplyTo pins that InReplyTo populates the context field.
func TestSendImageConfig_InReplyTo(t *testing.T) {
	cfg := NewSendImageByID("+16505551234", "media123").InReplyTo("wamid.ORIGINAL")
	require.NotNil(t, cfg.Context)
	assert.Equal(t, "wamid.ORIGINAL", cfg.Context.MessageID)
}

// TestClient_SendImageByID_requestShape pins the full outbound wire contract.
func TestClient_SendImageByID_requestShape(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{
			"messaging_product": "whatsapp",
			"contacts": [{"input": "+16505551234", "wa_id": "16505551234"}],
			"messages": [{"id": "wamid.IMG123"}]
		}`))
	}))
	defer ts.Close()

	resp, err := newTestClient(ts).SendImageByID(context.Background(), "+16505551234", "1479537139650973")
	require.NoError(t, err)
	assert.Equal(t, "wamid.IMG123", resp.MessageID())
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "image",
		"image": {"id": "1479537139650973"}
	}`, string(gotBody))
}

// TestClient_SendImageByLink_requestShape pins the link variant end to end.
func TestClient_SendImageByLink_requestShape(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{
			"messaging_product": "whatsapp",
			"contacts": [{"input": "+16505551234", "wa_id": "16505551234"}],
			"messages": [{"id": "wamid.IMG456"}]
		}`))
	}))
	defer ts.Close()

	resp, err := newTestClient(ts).SendImageByLink(
		context.Background(), "+16505551234", "https://example.com/photo.jpg")
	require.NoError(t, err)
	assert.Equal(t, "wamid.IMG456", resp.MessageID())
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "image",
		"image": {"link": "https://example.com/photo.jpg"}
	}`, string(gotBody))
}
