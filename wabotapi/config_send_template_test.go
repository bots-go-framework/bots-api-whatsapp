package wabotapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendTemplateConfigMarshal_namedParams pins the payload against Meta's own
// documented example for named-format templates, reproduced verbatim from
// developers.facebook.com/documentation/business-messaging/whatsapp/message-templates
func TestSendTemplateConfigMarshal_namedParams(t *testing.T) {
	cfg := NewSendTemplate("+16505551234", "order_confirmation", "en_US").
		WithNamedBodyParams(
			NamedParam{Name: "first_name", Value: "Jessica"},
			NamedParam{Name: "order_number", Value: "SKBUP2-4CPIG9"},
		)
	data, err := encodeToJson(cfg)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "template",
		"template": {
			"name": "order_confirmation",
			"language": {"code": "en_US"},
			"components": [
				{
					"type": "body",
					"parameters": [
						{"type": "text", "parameter_name": "first_name", "text": "Jessica"},
						{"type": "text", "parameter_name": "order_number", "text": "SKBUP2-4CPIG9"}
					]
				}
			]
		}
	}`, string(data))
}

// TestSendTemplateConfigMarshal_positionalParams pins Meta's positional-format
// example: same template, no parameter_name keys.
func TestSendTemplateConfigMarshal_positionalParams(t *testing.T) {
	cfg := NewSendTemplate("+16505551234", "order_confirmation", "en_US").
		WithBodyParams("Jessica", "SKBUP2-4CPIG9")
	data, err := encodeToJson(cfg)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+16505551234",
		"type": "template",
		"template": {
			"name": "order_confirmation",
			"language": {"code": "en_US"},
			"components": [
				{
					"type": "body",
					"parameters": [
						{"type": "text", "text": "Jessica"},
						{"type": "text", "text": "SKBUP2-4CPIG9"}
					]
				}
			]
		}
	}`, string(data))
	assert.NotContains(t, string(data), "parameter_name",
		"positional parameters must not emit parameter_name")
}

// TestSendTemplateConfigMarshal_noParams pins a template with no placeholders,
// which must omit components entirely rather than send an empty array.
func TestSendTemplateConfigMarshal_noParams(t *testing.T) {
	data, err := encodeToJson(NewSendTemplate("16505551234", "hello_world", "en_US"))
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "16505551234",
		"type": "template",
		"template": {
			"name": "hello_world",
			"language": {"code": "en_US"}
		}
	}`, string(data))
	assert.NotContains(t, string(data), "components")
}

// TestSendTemplateConfig_languageOmitsPolicy pins that the deprecated language
// `policy` field is never emitted. It is absent from Meta's current payloads.
func TestSendTemplateConfig_languageOmitsPolicy(t *testing.T) {
	data, err := encodeToJson(NewSendTemplate("16505551234", "hello_world", "en_US"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "policy")
}

func TestSendTemplateConfig_Endpoint(t *testing.T) {
	cfg := NewSendTemplate("16505551234", "hello_world", "en_US")
	assert.Equal(t, "1234567890/messages", cfg.Endpoint("1234567890"),
		"templates post to the same endpoint as any other message")
}

func TestSendTemplateConfig_Validate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cfg     *SendTemplateConfig
		wantErr error
	}{
		{
			name: "valid with no params",
			cfg:  NewSendTemplate("16505551234", "hello_world", "en_US"),
		},
		{
			name: "valid with positional params",
			cfg:  NewSendTemplate("16505551234", "order_confirmation", "en_US").WithBodyParams("a", "b"),
		},
		{
			name:    "no recipient",
			cfg:     NewSendTemplate("", "hello_world", "en_US"),
			wantErr: ErrNoRecipient,
		},
		{
			name:    "no template name",
			cfg:     NewSendTemplate("16505551234", "", "en_US"),
			wantErr: ErrNoTemplateName,
		},
		{
			name:    "no language code",
			cfg:     NewSendTemplate("16505551234", "hello_world", ""),
			wantErr: ErrNoTemplateLanguage,
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

// TestSendTemplateConfig_rejectsMixedParameterFormats pins Meta's rule that a
// component picks one parameter format. Mixing them is caught locally rather
// than costing a round trip and a 132018 template validation error.
func TestSendTemplateConfig_rejectsMixedParameterFormats(t *testing.T) {
	cfg := NewSendTemplate("16505551234", "order_confirmation", "en_US")
	cfg.Template.Components = []TemplateComponent{{
		Type: TemplateComponentBody,
		Parameters: []TemplateParameter{
			{Type: TemplateParameterText, ParameterName: "first_name", Text: "Jessica"},
			{Type: TemplateParameterText, Text: "SKBUP2-4CPIG9"},
		},
	}}
	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMixedParameterFormats)
}

// TestSendTemplate_overTheWire pins the full request through the client.
func TestSendTemplate_overTheWire(t *testing.T) {
	var gotPath string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{
			"messaging_product": "whatsapp",
			"contacts": [{"input": "16505551234", "wa_id": "16505551234"}],
			"messages": [{"id": "wamid.TPL1"}]
		}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	resp, err := c.SendTemplate(context.Background(), "16505551234", "order_confirmation", "en_US", "Jessica")
	require.NoError(t, err)

	assert.Equal(t, "/"+DefaultGraphVersion+"/1234567890/messages", gotPath)
	assert.Equal(t, "wamid.TPL1", resp.MessageID())
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "16505551234",
		"type": "template",
		"template": {
			"name": "order_confirmation",
			"language": {"code": "en_US"},
			"components": [
				{"type": "body", "parameters": [{"type": "text", "text": "Jessica"}]}
			]
		}
	}`, string(gotBody))
}

// TestSendTemplate_recoversFromReEngagement documents the intended flow: a
// free-form send outside the 24h window fails with 131047, and the remediation
// Meta documents is to send a template instead.
func TestSendTemplate_recoversFromReEngagement(t *testing.T) {
	var sawTemplate bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !sawTemplate {
			sawTemplate = true
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"(#131047) Re-engagement message","code":131047,"error_data":{"messaging_product":"whatsapp","details":"Message failed to send because more than 24 hours have passed since the customer last replied to this number."}}}`))
			return
		}
		require.Contains(t, string(body), `"type":"template"`)
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.TPL"}]}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)

	_, err := c.SendText(context.Background(), "16505551234", "hi there")
	require.Error(t, err)
	require.True(t, AsAPIError(err).IsReEngagementRequired())

	// The documented remediation: send a template instead.
	resp, err := c.SendTemplate(context.Background(), "16505551234", "hello_world", "en_US")
	require.NoError(t, err)
	assert.Equal(t, "wamid.TPL", resp.MessageID())
}

// TestReEngagement_classifiedByCodeNotDetailsText pins that the 24h-window branch
// keys on the integer code, not on `details`. Meta's documented wording and the
// actual wire wording differ, so any string match would be fragile.
func TestReEngagement_classifiedByCodeNotDetailsText(t *testing.T) {
	// Wire wording, as observed in real payloads.
	wire := &APIError{
		Code: ErrCodeReEngagementRequired,
		ErrorData: &APIErrorData{
			Details: "Message failed to send because more than 24 hours have passed since the customer last replied to this number.",
		},
	}
	// Documented wording, which differs.
	documented := &APIError{
		Code: ErrCodeReEngagementRequired,
		ErrorData: &APIErrorData{
			Details: "More than 24 hours have passed since the recipient last replied to the sender number.",
		},
	}
	assert.True(t, wire.IsReEngagementRequired())
	assert.True(t, documented.IsReEngagementRequired())
}
