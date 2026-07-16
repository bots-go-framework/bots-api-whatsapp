package wabotapi

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"
)

// CTA URL button message limits.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-cta-url-messages
const (
	// MaxCtaDisplayTextLength caps the button's visible label.
	MaxCtaDisplayTextLength = 20

	// MaxCtaBodyLength caps the body text of a CTA URL button message.
	MaxCtaBodyLength = 1024

	// MaxCtaHeaderTextLength caps a text-type header.
	MaxCtaHeaderTextLength = 60

	// MaxCtaFooterLength caps the footer of a CTA URL button message.
	MaxCtaFooterLength = 60
)

// CTA URL button errors.
var (
	// ErrCtaDisplayTextEmpty means the button label was blank.
	ErrCtaDisplayTextEmpty = errors.New("cta_url button display_text must not be empty")

	// ErrCtaURLEmpty means the button URL was blank.
	ErrCtaURLEmpty = errors.New("cta_url button url must not be empty")
)

// InteractiveTypeCTAURL is the interactive type value for CTA URL messages.
const InteractiveTypeCTAURL = InteractiveType("cta_url")

// CtaURLParameters carries the button label and URL for a CTA URL action.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-cta-url-messages
type CtaURLParameters struct {
	// DisplayText is the visible button label. Maximum MaxCtaDisplayTextLength characters.
	DisplayText string `json:"display_text"`

	// URL is the URL opened when the button is tapped.
	URL string `json:"url"`
}

// CtaURLAction is the action object of a CTA URL interactive message.
//
// Name must always be "cta_url".
type CtaURLAction struct {
	// Name is always "cta_url".
	Name string `json:"name"`

	// Parameters carries the display text and URL.
	Parameters CtaURLParameters `json:"parameters"`
}

// InteractiveCTAURL is the interactive object of a CTA URL button message.
type InteractiveCTAURL struct {
	Type   InteractiveType    `json:"type"`
	Header *InteractiveHeader `json:"header,omitempty"`
	Body   InteractiveBody    `json:"body"`
	Footer *InteractiveFooter `json:"footer,omitempty"`
	Action CtaURLAction       `json:"action"`
}

var _ Sendable = (*SendCTAURLConfig)(nil)

// SendCTAURLConfig sends an interactive CTA URL button message.
//
// A CTA URL message attaches one tappable button that opens a URL in the
// device's default browser. The body is required; a text header and footer
// are optional. The URL is fully dynamic per send, which makes this the
// in-window analogue of Telegram's per-message URL buttons.
//
// Note: this message type is only deliverable inside the 24-hour
// customer-service window. Outside it, use a pre-approved template instead.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/interactive-cta-url-messages
type SendCTAURLConfig struct {
	BaseMessage
	Interactive InteractiveCTAURL `json:"interactive"`
}

// NewSendCTAURL builds a CTA URL button message.
//
// displayText is the button label (max MaxCtaDisplayTextLength chars).
// url is the URL the button opens.
// body is the required message body (max MaxCtaBodyLength chars).
func NewSendCTAURL(to, body, displayText, url string) *SendCTAURLConfig {
	return &SendCTAURLConfig{
		BaseMessage: newBaseMessage(to, MessageTypeInteractive),
		Interactive: InteractiveCTAURL{
			Type: InteractiveTypeCTAURL,
			Body: InteractiveBody{Text: body},
			Action: CtaURLAction{
				Name: "cta_url",
				Parameters: CtaURLParameters{
					DisplayText: displayText,
					URL:         url,
				},
			},
		},
	}
}

// WithTextHeader sets an optional plain-text header (max MaxCtaHeaderTextLength chars).
func (c *SendCTAURLConfig) WithTextHeader(text string) *SendCTAURLConfig {
	c.Interactive.Header = &InteractiveHeader{Type: "text", Text: text}
	return c
}

// WithFooter sets an optional footer (max MaxCtaFooterLength chars).
func (c *SendCTAURLConfig) WithFooter(text string) *SendCTAURLConfig {
	c.Interactive.Footer = &InteractiveFooter{Text: text}
	return c
}

// Validate implements Sendable.
func (c *SendCTAURLConfig) Validate() error {
	if err := c.BaseMessage.Validate(); err != nil {
		return err
	}
	if c.Interactive.Body.Text == "" {
		return ErrEmptyBody
	}
	if n := utf8.RuneCountInString(c.Interactive.Body.Text); n > MaxCtaBodyLength {
		return fmt.Errorf("cta_url body is %d characters, max is %d", n, MaxCtaBodyLength)
	}
	if c.Interactive.Action.Parameters.DisplayText == "" {
		return ErrCtaDisplayTextEmpty
	}
	if n := utf8.RuneCountInString(c.Interactive.Action.Parameters.DisplayText); n > MaxCtaDisplayTextLength {
		return fmt.Errorf("cta_url display_text is %d characters, max is %d", n, MaxCtaDisplayTextLength)
	}
	if c.Interactive.Action.Parameters.URL == "" {
		return ErrCtaURLEmpty
	}
	if c.Interactive.Footer != nil {
		if n := utf8.RuneCountInString(c.Interactive.Footer.Text); n > MaxCtaFooterLength {
			return fmt.Errorf("cta_url footer is %d characters, max is %d", n, MaxCtaFooterLength)
		}
	}
	if c.Interactive.Header != nil && c.Interactive.Header.Type == "text" {
		if n := utf8.RuneCountInString(c.Interactive.Header.Text); n > MaxCtaHeaderTextLength {
			return fmt.Errorf("cta_url text header is %d characters, max is %d", n, MaxCtaHeaderTextLength)
		}
	}
	return nil
}

// SendCTAURL is a convenience wrapper for a CTA URL button message.
func (c *Client) SendCTAURL(
	ctx context.Context, to, body, displayText, url string,
) (*SendMessageResponse, error) {
	return c.SendMessage(ctx, NewSendCTAURL(to, body, displayText, url))
}
