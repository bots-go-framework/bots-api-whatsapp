package wabotapi

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Interactive message limits.
//
// These are hard caps from Meta's reference, not guidance. Exceeding them earns a
// 400; validating locally is cheaper.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-reply-buttons-messages
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-list-messages
const (
	// MaxReplyButtons is the hard cap on reply buttons in one message.
	//
	// Three. Flat — there is no grid or row concept. This is the single biggest
	// structural gap versus Telegram's arbitrarily sized inline keyboards.
	MaxReplyButtons = 3

	// MaxButtonIDLength caps a reply button's id — the callback payload.
	//
	// 256 chars, comfortably more than Telegram's 64-byte callback_data cap, so
	// callback URLs port without truncation.
	MaxButtonIDLength = 256

	// MaxButtonTitleLength caps a reply button's visible label. Titles must also
	// be unique within a message.
	MaxButtonTitleLength = 20

	// MaxButtonsBodyLength caps the body of a reply-buttons message.
	MaxButtonsBodyLength = 1024

	// MaxFooterLength caps the footer of any interactive message.
	MaxFooterLength = 60

	// MaxListSections caps sections in a list message.
	MaxListSections = 10

	// MaxListRows caps rows ACROSS ALL SECTIONS COMBINED — not per section.
	// Sections group visually; they do not raise the ceiling.
	MaxListRows = 10

	// MaxListBodyLength caps the body of a list message.
	MaxListBodyLength = 4096

	// MaxListHeaderLength caps a list header, which is text-only.
	MaxListHeaderLength = 60

	// MaxListButtonLength caps the label of the button that reveals the list.
	MaxListButtonLength = 20

	// MaxRowIDLength caps a list row's id — the callback payload.
	MaxRowIDLength = 200

	// MaxRowTitleLength caps a list row's title.
	MaxRowTitleLength = 24

	// MaxRowDescriptionLength caps a list row's optional description.
	MaxRowDescriptionLength = 72
)

// Interactive message errors.
var (
	// ErrTooManyButtons means more than MaxReplyButtons were supplied.
	ErrTooManyButtons = fmt.Errorf("a message may carry at most %d reply buttons", MaxReplyButtons)

	// ErrDuplicateButtonTitle means two buttons shared a label, which Meta rejects.
	ErrDuplicateButtonTitle = errors.New("reply button titles must be unique within a message")

	// ErrNoButtons means an interactive button message carried no buttons.
	ErrNoButtons = errors.New("at least one reply button is required")

	// ErrTooManyRows means more than MaxListRows were supplied across all sections.
	ErrTooManyRows = fmt.Errorf("a list may carry at most %d rows across all sections", MaxListRows)

	// ErrNoRows means a list message carried no rows.
	ErrNoRows = errors.New("at least one list row is required")
)

// InteractiveType discriminates interactive message kinds.
type InteractiveType string

// Interactive message types.
const (
	InteractiveTypeButton = InteractiveType("button")
	InteractiveTypeList   = InteractiveType("list")
)

// InteractiveBody is the body text of an interactive message.
type InteractiveBody struct {
	Text string `json:"text"`
}

// InteractiveFooter is the optional footer of an interactive message.
type InteractiveFooter struct {
	Text string `json:"text"`
}

// InteractiveHeader is the optional header of an interactive message.
//
// Only the text type is modelled. Reply buttons also accept image/video/document
// headers, but their nested shapes are not verified yet.
type InteractiveHeader struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ReplyButton is one tappable button.
type ReplyButton struct {
	// Type is always "reply".
	Type string `json:"type"`

	Reply ReplyButtonPayload `json:"reply"`
}

// ReplyButtonPayload carries a button's id and label.
type ReplyButtonPayload struct {
	// ID is echoed back in the inbound webhook when tapped — the callback payload.
	ID string `json:"id"`

	// Title is the visible label.
	Title string `json:"title"`
}

// NewReplyButton builds a reply button.
func NewReplyButton(id, title string) ReplyButton {
	return ReplyButton{Type: "reply", Reply: ReplyButtonPayload{ID: id, Title: title}}
}

// ButtonsAction carries a reply-buttons message's buttons.
type ButtonsAction struct {
	Buttons []ReplyButton `json:"buttons"`
}

// InteractiveButtons is the interactive object of a reply-buttons message.
type InteractiveButtons struct {
	Type   InteractiveType    `json:"type"`
	Header *InteractiveHeader `json:"header,omitempty"`
	Body   InteractiveBody    `json:"body"`
	Footer *InteractiveFooter `json:"footer,omitempty"`
	Action ButtonsAction      `json:"action"`
}

var _ Sendable = (*SendButtonsConfig)(nil)

// SendButtonsConfig sends up to MaxReplyButtons tappable reply buttons.
//
// A tap arrives as an ordinary inbound message webhook echoing the button id.
// There is no callback-acknowledgement step — no answerCallbackQuery analogue.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-reply-buttons-messages
type SendButtonsConfig struct {
	BaseMessage
	Interactive InteractiveButtons `json:"interactive"`
}

// NewSendButtons builds a reply-buttons message.
func NewSendButtons(to, body string, buttons ...ReplyButton) *SendButtonsConfig {
	return &SendButtonsConfig{
		BaseMessage: newBaseMessage(to, MessageTypeInteractive),
		Interactive: InteractiveButtons{
			Type:   InteractiveTypeButton,
			Body:   InteractiveBody{Text: body},
			Action: ButtonsAction{Buttons: buttons},
		},
	}
}

// WithFooter sets the optional footer.
func (c *SendButtonsConfig) WithFooter(text string) *SendButtonsConfig {
	c.Interactive.Footer = &InteractiveFooter{Text: text}
	return c
}

// Validate implements Sendable.
func (c *SendButtonsConfig) Validate() error {
	if err := c.BaseMessage.Validate(); err != nil {
		return err
	}
	if c.Interactive.Body.Text == "" {
		return ErrEmptyBody
	}
	if n := utf8.RuneCountInString(c.Interactive.Body.Text); n > MaxButtonsBodyLength {
		return fmt.Errorf("body is %d characters, max is %d", n, MaxButtonsBodyLength)
	}
	btns := c.Interactive.Action.Buttons
	if len(btns) == 0 {
		return ErrNoButtons
	}
	if len(btns) > MaxReplyButtons {
		return fmt.Errorf("%w, got %d", ErrTooManyButtons, len(btns))
	}
	seen := make(map[string]bool, len(btns))
	for _, b := range btns {
		if b.Reply.ID == "" {
			return errors.New("reply button id must not be empty")
		}
		if n := utf8.RuneCountInString(b.Reply.ID); n > MaxButtonIDLength {
			return fmt.Errorf("button id is %d characters, max is %d", n, MaxButtonIDLength)
		}
		if b.Reply.Title == "" {
			return errors.New("reply button title must not be empty")
		}
		if n := utf8.RuneCountInString(b.Reply.Title); n > MaxButtonTitleLength {
			return fmt.Errorf("button title %q is %d characters, max is %d", b.Reply.Title, n, MaxButtonTitleLength)
		}
		if seen[b.Reply.Title] {
			return fmt.Errorf("%w: %q", ErrDuplicateButtonTitle, b.Reply.Title)
		}
		seen[b.Reply.Title] = true
	}
	if c.Interactive.Footer != nil {
		if n := utf8.RuneCountInString(c.Interactive.Footer.Text); n > MaxFooterLength {
			return fmt.Errorf("footer is %d characters, max is %d", n, MaxFooterLength)
		}
	}
	return nil
}

// ListRow is one selectable row in a list message.
type ListRow struct {
	// ID is echoed back in the inbound webhook when selected.
	ID string `json:"id"`

	Title string `json:"title"`

	Description string `json:"description,omitempty"`
}

// ListSection groups rows under a title. Sections group visually only — they do
// not raise the MaxListRows ceiling.
type ListSection struct {
	Title string `json:"title,omitempty"`

	Rows []ListRow `json:"rows"`
}

// ListAction carries a list message's reveal button and sections.
type ListAction struct {
	// Button is the label of the button that reveals the list.
	Button string `json:"button"`

	Sections []ListSection `json:"sections"`
}

// InteractiveList is the interactive object of a list message.
type InteractiveList struct {
	Type   InteractiveType    `json:"type"`
	Header *InteractiveHeader `json:"header,omitempty"`
	Body   InteractiveBody    `json:"body"`
	Footer *InteractiveFooter `json:"footer,omitempty"`
	Action ListAction         `json:"action"`
}

var _ Sendable = (*SendListConfig)(nil)

// SendListConfig sends a tap-to-reveal list of up to MaxListRows options.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-list-messages
type SendListConfig struct {
	BaseMessage
	Interactive InteractiveList `json:"interactive"`
}

// NewSendList builds a list message. buttonText labels the reveal button.
func NewSendList(to, body, buttonText string, sections ...ListSection) *SendListConfig {
	return &SendListConfig{
		BaseMessage: newBaseMessage(to, MessageTypeInteractive),
		Interactive: InteractiveList{
			Type:   InteractiveTypeList,
			Body:   InteractiveBody{Text: body},
			Action: ListAction{Button: buttonText, Sections: sections},
		},
	}
}

// WithHeader sets the optional text header. List headers are text-only.
func (c *SendListConfig) WithHeader(text string) *SendListConfig {
	c.Interactive.Header = &InteractiveHeader{Type: "text", Text: text}
	return c
}

// Validate implements Sendable.
func (c *SendListConfig) Validate() error {
	if err := c.BaseMessage.Validate(); err != nil {
		return err
	}
	if c.Interactive.Body.Text == "" {
		return ErrEmptyBody
	}
	if n := utf8.RuneCountInString(c.Interactive.Body.Text); n > MaxListBodyLength {
		return fmt.Errorf("body is %d characters, max is %d", n, MaxListBodyLength)
	}
	if c.Interactive.Action.Button == "" {
		return errors.New("list button text must not be empty")
	}
	if n := utf8.RuneCountInString(c.Interactive.Action.Button); n > MaxListButtonLength {
		return fmt.Errorf("list button text is %d characters, max is %d", n, MaxListButtonLength)
	}
	sections := c.Interactive.Action.Sections
	if len(sections) == 0 {
		return ErrNoRows
	}
	if len(sections) > MaxListSections {
		return fmt.Errorf("a list may carry at most %d sections, got %d", MaxListSections, len(sections))
	}

	var totalRows int
	for _, s := range sections {
		totalRows += len(s.Rows)
		for _, r := range s.Rows {
			if r.ID == "" {
				return errors.New("list row id must not be empty")
			}
			if n := utf8.RuneCountInString(r.ID); n > MaxRowIDLength {
				return fmt.Errorf("row id is %d characters, max is %d", n, MaxRowIDLength)
			}
			if r.Title == "" {
				return errors.New("list row title must not be empty")
			}
			if n := utf8.RuneCountInString(r.Title); n > MaxRowTitleLength {
				return fmt.Errorf("row title %q is %d characters, max is %d", r.Title, n, MaxRowTitleLength)
			}
			if n := utf8.RuneCountInString(r.Description); n > MaxRowDescriptionLength {
				return fmt.Errorf("row description is %d characters, max is %d", n, MaxRowDescriptionLength)
			}
		}
	}
	if totalRows == 0 {
		return ErrNoRows
	}
	// The cap is across ALL sections combined, which is the easy thing to get wrong.
	if totalRows > MaxListRows {
		return fmt.Errorf("%w, got %d", ErrTooManyRows, totalRows)
	}
	if c.Interactive.Header != nil {
		if n := utf8.RuneCountInString(c.Interactive.Header.Text); n > MaxListHeaderLength {
			return fmt.Errorf("header is %d characters, max is %d", n, MaxListHeaderLength)
		}
	}
	return nil
}

// SendButtons is a convenience wrapper for a reply-buttons message.
func (c *Client) SendButtons(
	ctx context.Context, to, body string, buttons ...ReplyButton,
) (*SendMessageResponse, error) {
	return c.SendMessage(ctx, NewSendButtons(to, body, buttons...))
}

// SendList is a convenience wrapper for a list message.
func (c *Client) SendList(
	ctx context.Context, to, body, buttonText string, sections ...ListSection,
) (*SendMessageResponse, error) {
	return c.SendMessage(ctx, NewSendList(to, body, buttonText, sections...))
}
