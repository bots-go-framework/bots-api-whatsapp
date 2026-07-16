package wabotapi

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendButtonsConfigMarshal pins the payload against Meta's documented request
// syntax for interactive reply buttons.
func TestSendButtonsConfigMarshal(t *testing.T) {
	cfg := NewSendButtons("16505551234", "Need to change anything?",
		NewReplyButton("change-button", "Change"),
		NewReplyButton("cancel-button", "Cancel"),
	).WithFooter("Lucky Shrub")

	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "16505551234",
		"type": "interactive",
		"interactive": {
			"type": "button",
			"body": {"text": "Need to change anything?"},
			"footer": {"text": "Lucky Shrub"},
			"action": {
				"buttons": [
					{"type": "reply", "reply": {"id": "change-button", "title": "Change"}},
					{"type": "reply", "reply": {"id": "cancel-button", "title": "Cancel"}}
				]
			}
		}
	}`, string(data))
}

func TestSendButtonsConfig_Validate(t *testing.T) {
	valid := func() *SendButtonsConfig {
		return NewSendButtons("16505551234", "pick", NewReplyButton("a", "A"))
	}

	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, valid().Validate())
	})

	// The single biggest structural gap versus Telegram's arbitrary inline grids.
	t.Run("at most 3 buttons", func(t *testing.T) {
		three := NewSendButtons("16505551234", "pick",
			NewReplyButton("a", "A"), NewReplyButton("b", "B"), NewReplyButton("c", "C"))
		assert.NoError(t, three.Validate(), "3 is the cap and must pass")

		four := NewSendButtons("16505551234", "pick",
			NewReplyButton("a", "A"), NewReplyButton("b", "B"),
			NewReplyButton("c", "C"), NewReplyButton("d", "D"))
		assert.ErrorIs(t, four.Validate(), ErrTooManyButtons)
	})

	t.Run("titles must be unique", func(t *testing.T) {
		cfg := NewSendButtons("16505551234", "pick",
			NewReplyButton("a", "Same"), NewReplyButton("b", "Same"))
		assert.ErrorIs(t, cfg.Validate(), ErrDuplicateButtonTitle)
	})

	t.Run("no buttons", func(t *testing.T) {
		assert.ErrorIs(t, NewSendButtons("16505551234", "pick").Validate(), ErrNoButtons)
	})

	t.Run("title length", func(t *testing.T) {
		cfg := NewSendButtons("16505551234", "pick",
			NewReplyButton("a", strings.Repeat("x", MaxButtonTitleLength+1)))
		require.Error(t, cfg.Validate())
		assert.Contains(t, cfg.Validate().Error(), "max is 20")
	})

	// The good news: 256 chars comfortably exceeds Telegram's 64-byte callback_data
	// cap, so callback URLs port without truncation.
	t.Run("button id allows 256 chars", func(t *testing.T) {
		atCap := NewSendButtons("16505551234", "pick",
			NewReplyButton(strings.Repeat("x", MaxButtonIDLength), "A"))
		assert.NoError(t, atCap.Validate(), "256 is the cap and must pass")

		over := NewSendButtons("16505551234", "pick",
			NewReplyButton(strings.Repeat("x", MaxButtonIDLength+1), "A"))
		assert.Error(t, over.Validate())
	})

	t.Run("no recipient", func(t *testing.T) {
		cfg := NewSendButtons("", "pick", NewReplyButton("a", "A"))
		assert.ErrorIs(t, cfg.Validate(), ErrNoRecipient)
	})
}

// TestSendListConfigMarshal pins the payload against Meta's documented list syntax.
func TestSendListConfigMarshal(t *testing.T) {
	cfg := NewSendList("16505551234", "Which shipping option do you prefer?", "Shipping Options",
		ListSection{
			Title: "I want it ASAP!",
			Rows: []ListRow{
				{ID: "priority_express", Title: "Priority Mail Express", Description: "Next Day to 2 Days"},
			},
		},
	).WithHeader("Choose Shipping Option")

	data, err := encodeToJson(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "16505551234",
		"type": "interactive",
		"interactive": {
			"type": "list",
			"header": {"type": "text", "text": "Choose Shipping Option"},
			"body": {"text": "Which shipping option do you prefer?"},
			"action": {
				"button": "Shipping Options",
				"sections": [
					{
						"title": "I want it ASAP!",
						"rows": [
							{"id": "priority_express", "title": "Priority Mail Express", "description": "Next Day to 2 Days"}
						]
					}
				]
			}
		}
	}`, string(data))
}

// TestSendListConfig_rowCapIsAcrossAllSections pins the trap: 10 rows is the total
// across every section, not per section. Sections group visually; they do not raise
// the ceiling.
func TestSendListConfig_rowCapIsAcrossAllSections(t *testing.T) {
	rows := func(n int, prefix string) []ListRow {
		out := make([]ListRow, n)
		for i := range out {
			out[i] = ListRow{ID: prefix + string(rune('a'+i)), Title: prefix + string(rune('a'+i))}
		}
		return out
	}

	// 2 sections x 5 rows = 10 total: at the cap, must pass.
	ok := NewSendList("16505551234", "pick", "Options",
		ListSection{Title: "One", Rows: rows(5, "x")},
		ListSection{Title: "Two", Rows: rows(5, "y")},
	)
	assert.NoError(t, ok.Validate(), "10 rows across 2 sections is exactly the cap")

	// 2 sections x 6 rows = 12 total. Each section is small, but the TOTAL is over.
	over := NewSendList("16505551234", "pick", "Options",
		ListSection{Title: "One", Rows: rows(6, "x")},
		ListSection{Title: "Two", Rows: rows(6, "y")},
	)
	err := over.Validate()
	assert.ErrorIs(t, err, ErrTooManyRows,
		"the cap is across all sections combined, not per section")
	assert.Contains(t, err.Error(), "got 12")
}

func TestSendListConfig_Validate(t *testing.T) {
	section := ListSection{Title: "S", Rows: []ListRow{{ID: "r1", Title: "Row 1"}}}

	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, NewSendList("16505551234", "pick", "Options", section).Validate())
	})

	t.Run("no sections", func(t *testing.T) {
		assert.ErrorIs(t, NewSendList("16505551234", "pick", "Options").Validate(), ErrNoRows)
	})

	t.Run("no button text", func(t *testing.T) {
		err := NewSendList("16505551234", "pick", "", section).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "button text")
	})

	t.Run("row title length", func(t *testing.T) {
		long := ListSection{Rows: []ListRow{{ID: "r", Title: strings.Repeat("x", MaxRowTitleLength+1)}}}
		err := NewSendList("16505551234", "pick", "Options", long).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 24")
	})

	t.Run("row description length", func(t *testing.T) {
		long := ListSection{Rows: []ListRow{
			{ID: "r", Title: "T", Description: strings.Repeat("x", MaxRowDescriptionLength+1)},
		}}
		err := NewSendList("16505551234", "pick", "Options", long).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max is 72")
	})

	t.Run("too many sections", func(t *testing.T) {
		var many []ListSection
		for i := 0; i < MaxListSections+1; i++ {
			many = append(many, ListSection{Rows: []ListRow{{ID: "r", Title: "T"}}})
		}
		err := NewSendList("16505551234", "pick", "Options", many...).Validate()
		require.Error(t, err)
		// 11 sections x 1 row = 11 rows, so the row cap trips too; either is correct.
		assert.True(t,
			errors.Is(err, ErrTooManyRows) || strings.Contains(err.Error(), "sections"),
			"got %v", err)
	})
}

func TestSendInteractive_Endpoint(t *testing.T) {
	assert.Equal(t, "1234567890/messages",
		NewSendButtons("16505551234", "b", NewReplyButton("a", "A")).Endpoint("1234567890"))
	assert.Equal(t, "1234567890/messages",
		NewSendList("16505551234", "b", "Btn", ListSection{Rows: []ListRow{{ID: "r", Title: "T"}}}).Endpoint("1234567890"))
}
