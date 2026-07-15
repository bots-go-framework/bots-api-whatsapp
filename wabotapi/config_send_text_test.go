package wabotapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encodeToJson marshals v to compact JSON, mirroring the helper convention in
// bots-api-telegram's api_v*_test.go files.
func encodeToJson(v any) ([]byte, error) {
	return json.Marshal(v)
}

// TestSendTextConfigMarshal pins the minimal text-message wire payload.
func TestSendTextConfigMarshal(t *testing.T) {
	cfg := NewSendText("16505551234", "hello")
	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "16505551234",
		"type": "text",
		"text": {"body": "hello"}
	}`, string(data))
}

// TestSendTextConfigMarshal_previewURLOmittedWhenFalse pins that preview_url is
// absent rather than explicitly false. Safe here only because the API default is
// false, so unset and false are equivalent.
func TestSendTextConfigMarshal_previewURLOmittedWhenFalse(t *testing.T) {
	data, err := encodeToJson(NewSendText("16505551234", "hello"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "preview_url")
}

// TestSendTextConfigMarshal_withPreviewURL pins the opt-in link preview.
func TestSendTextConfigMarshal_withPreviewURL(t *testing.T) {
	cfg := NewSendText("16505551234", "see https://example.com").WithPreviewURL()
	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "16505551234",
		"type": "text",
		"text": {"body": "see https://example.com", "preview_url": true}
	}`, string(data))
}

// TestSendTextConfigMarshal_inReplyTo pins the reply-threading context object.
func TestSendTextConfigMarshal_inReplyTo(t *testing.T) {
	cfg := NewSendText("16505551234", "replying").InReplyTo("wamid.ORIGINAL")
	data, err := encodeToJson(cfg)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))
	require.Contains(t, got, "context")
	assert.Equal(t, map[string]any{"message_id": "wamid.ORIGINAL"}, got["context"])
}

// TestSendTextConfig_contextOmittedByDefault pins that a non-reply carries no
// context object.
func TestSendTextConfig_contextOmittedByDefault(t *testing.T) {
	data, err := encodeToJson(NewSendText("16505551234", "hi"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "context")
}

func TestSendTextConfig_Endpoint(t *testing.T) {
	cfg := NewSendText("16505551234", "hi")
	assert.Equal(t, "1234567890/messages", cfg.Endpoint("1234567890"))
}

func TestSendTextConfig_Validate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cfg     *SendTextConfig
		wantErr error
	}{
		{
			name: "valid",
			cfg:  NewSendText("16505551234", "hi"),
		},
		{
			name:    "no recipient",
			cfg:     NewSendText("", "hi"),
			wantErr: ErrNoRecipient,
		},
		{
			name:    "empty body",
			cfg:     NewSendText("16505551234", ""),
			wantErr: ErrEmptyBody,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestSendTextConfig_ValidateBodyLength pins the 4096-character limit, counted in
// runes rather than bytes so multi-byte text is not rejected early.
func TestSendTextConfig_ValidateBodyLength(t *testing.T) {
	atLimit := NewSendText("16505551234", strings.Repeat("a", MaxTextBodyLength))
	assert.NoError(t, atLimit.Validate())

	overLimit := NewSendText("16505551234", strings.Repeat("a", MaxTextBodyLength+1))
	require.Error(t, overLimit.Validate())
	assert.Contains(t, overLimit.Validate().Error(), "max is 4096")

	// 4096 multi-byte runes is 12288 bytes but is still within the limit.
	multiByte := NewSendText("16505551234", strings.Repeat("日", MaxTextBodyLength))
	assert.NoError(t, multiByte.Validate(),
		"the limit is characters, not bytes")
}
