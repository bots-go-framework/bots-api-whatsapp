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

// TestGetConversationalAutomation_requestShape pins the GET wire contract:
// method, path (phone-number-id?fields=conversational_automation), auth header,
// and response decoding.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
func TestGetConversationalAutomation_requestShape(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{
			"conversational_automation": {
				"commands": [
					{"command_name": "order_status", "command_description": "Check your order status"},
					{"command_name": "help", "command_description": "Get help"}
				],
				"prompts": [
					"What are your hours?",
					"Do you deliver?"
				]
			},
			"id": "1234567890"
		}`))
	}))
	defer ts.Close()

	resp, err := newTestClient(ts).GetConversationalAutomation(context.Background())
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Contains(t, gotPath, "1234567890")
	assert.Contains(t, gotPath, "fields=conversational_automation")

	require.Len(t, resp.ConversationalAutomation.Commands, 2)
	assert.Equal(t, "order_status", resp.ConversationalAutomation.Commands[0].Name)
	assert.Equal(t, "Check your order status", resp.ConversationalAutomation.Commands[0].Description)
	assert.Equal(t, "help", resp.ConversationalAutomation.Commands[1].Name)

	require.Len(t, resp.ConversationalAutomation.Prompts, 2)
	assert.Equal(t, "What are your hours?", resp.ConversationalAutomation.Prompts[0])
	assert.Equal(t, "Do you deliver?", resp.ConversationalAutomation.Prompts[1])

	assert.Equal(t, "1234567890", resp.ID)
}

// TestSetCommands_requestShape pins the POST wire contract for commands.
//
// The endpoint is /{phone-number-id}/conversational_automation and the body
// must carry a "commands" array of objects with command_name and command_description.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
func TestSetCommands_requestShape(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer ts.Close()

	err := newTestClient(ts).SetCommands(context.Background(), []Command{
		{Name: "order_status", Description: "Check your order status"},
		{Name: "help", Description: "Get help with anything"},
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/"+DefaultGraphVersion+"/1234567890/conversational_automation", gotPath)
	assert.JSONEq(t, `{
		"commands": [
			{"command_name": "order_status", "command_description": "Check your order status"},
			{"command_name": "help", "command_description": "Get help with anything"}
		]
	}`, string(gotBody))
}

// TestSetIceBreakers_requestShape pins the POST wire contract for ice breakers.
//
// The body must carry a "prompts" array of strings.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
func TestSetIceBreakers_requestShape(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer ts.Close()

	err := newTestClient(ts).SetIceBreakers(context.Background(), []string{
		"What are your hours?",
		"Do you deliver?",
		"How do I place an order?",
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/"+DefaultGraphVersion+"/1234567890/conversational_automation", gotPath)
	assert.JSONEq(t, `{
		"prompts": ["What are your hours?", "Do you deliver?", "How do I place an order?"]
	}`, string(gotBody))
}

// TestSetCommands_Validate covers all command validation branches.
func TestSetCommands_Validate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer ts.Close()
	c := newTestClient(ts)
	ctx := context.Background()

	t.Run("valid at max count", func(t *testing.T) {
		cmds := make([]Command, MaxCommands)
		for i := range cmds {
			cmds[i] = Command{Name: "cmd", Description: "desc"}
		}
		assert.NoError(t, c.SetCommands(ctx, cmds))
	})

	t.Run("too many commands", func(t *testing.T) {
		cmds := make([]Command, MaxCommands+1)
		for i := range cmds {
			cmds[i] = Command{Name: "cmd", Description: "desc"}
		}
		err := c.SetCommands(ctx, cmds)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most 30")
	})

	t.Run("empty command name", func(t *testing.T) {
		err := c.SetCommands(ctx, []Command{{Name: "", Description: "desc"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "command_name must not be empty")
	})

	t.Run("command name at cap", func(t *testing.T) {
		cmd := Command{Name: strings.Repeat("x", MaxCommandNameLength), Description: "desc"}
		assert.NoError(t, c.SetCommands(ctx, []Command{cmd}))
	})

	t.Run("command name over limit", func(t *testing.T) {
		cmd := Command{Name: strings.Repeat("x", MaxCommandNameLength+1), Description: "desc"}
		err := c.SetCommands(ctx, []Command{cmd})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 32")
	})

	t.Run("description over limit", func(t *testing.T) {
		cmd := Command{Name: "cmd", Description: strings.Repeat("x", MaxCommandDescriptionLength+1)}
		err := c.SetCommands(ctx, []Command{cmd})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 256")
	})

	t.Run("empty slice clears commands", func(t *testing.T) {
		// An empty slice is valid — it clears the command list.
		assert.NoError(t, c.SetCommands(ctx, []Command{}))
	})
}

// TestSetIceBreakers_Validate covers all ice-breaker validation branches.
func TestSetIceBreakers_Validate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer ts.Close()
	c := newTestClient(ts)
	ctx := context.Background()

	t.Run("valid at max count", func(t *testing.T) {
		prompts := make([]string, MaxIceBreakers)
		for i := range prompts {
			prompts[i] = "prompt"
		}
		assert.NoError(t, c.SetIceBreakers(ctx, prompts))
	})

	t.Run("too many ice breakers", func(t *testing.T) {
		prompts := make([]string, MaxIceBreakers+1)
		for i := range prompts {
			prompts[i] = "prompt"
		}
		err := c.SetIceBreakers(ctx, prompts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most 4")
	})

	t.Run("empty prompt", func(t *testing.T) {
		err := c.SetIceBreakers(ctx, []string{"valid", ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("prompt at cap", func(t *testing.T) {
		p := strings.Repeat("x", MaxIceBreakerLength)
		assert.NoError(t, c.SetIceBreakers(ctx, []string{p}))
	})

	t.Run("prompt over limit", func(t *testing.T) {
		p := strings.Repeat("x", MaxIceBreakerLength+1)
		err := c.SetIceBreakers(ctx, []string{p})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 80")
	})

	t.Run("empty slice clears prompts", func(t *testing.T) {
		assert.NoError(t, c.SetIceBreakers(ctx, []string{}))
	})
}

// TestGetConversationalAutomation_emptyConfig covers the case where neither
// commands nor prompts are configured. The struct must decode cleanly with
// zero-value slices rather than erroring.
func TestGetConversationalAutomation_emptyConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"conversational_automation": {}, "id": "1234567890"}`))
	}))
	defer ts.Close()

	resp, err := newTestClient(ts).GetConversationalAutomation(context.Background())
	require.NoError(t, err)
	assert.Empty(t, resp.ConversationalAutomation.Commands)
	assert.Empty(t, resp.ConversationalAutomation.Prompts)
}

// TestGetConversationalAutomation_undecodableResponse pins the decode-failure branch.
func TestGetConversationalAutomation_undecodableResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`["not", "an", "object"]`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).GetConversationalAutomation(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}
