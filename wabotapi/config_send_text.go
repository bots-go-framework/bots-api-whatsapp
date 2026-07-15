package wabotapi

import (
	"context"
	"fmt"
	"unicode/utf8"
)

// MaxTextBodyLength is the Cloud API limit on a text message body, in characters.
//
// https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#text-object
const MaxTextBodyLength = 4096

var _ Sendable = (*SendTextConfig)(nil)

// SendTextConfig sends a plain text message.
//
// https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#text-object
type SendTextConfig struct {
	BaseMessage
	Text TextBody `json:"text"`
}

// TextBody is the text object of a text message.
type TextBody struct {
	// Body is the message text. Max MaxTextBodyLength characters.
	Body string `json:"body"`

	// PreviewURL renders a link preview for the first URL in Body.
	//
	// A plain bool is correct here: the API default is false, so "unset" and
	// "explicitly false" mean the same thing. Fields whose unset and false
	// behaviours differ must use *bool.
	PreviewURL bool `json:"preview_url,omitempty"`
}

// NewSendText builds a text message config for a recipient.
func NewSendText(to, body string) *SendTextConfig {
	return &SendTextConfig{
		BaseMessage: newBaseMessage(to, MessageTypeText),
		Text:        TextBody{Body: body},
	}
}

// WithPreviewURL enables link preview and returns the config for chaining.
func (c *SendTextConfig) WithPreviewURL() *SendTextConfig {
	c.Text.PreviewURL = true
	return c
}

// InReplyTo threads this message as a reply to the given wamid.
func (c *SendTextConfig) InReplyTo(messageID string) *SendTextConfig {
	c.Context = &MessageContext{MessageID: messageID}
	return c
}

// Validate implements Sendable.
func (c *SendTextConfig) Validate() error {
	if err := c.BaseMessage.Validate(); err != nil {
		return err
	}
	if c.Text.Body == "" {
		return ErrEmptyBody
	}
	if n := utf8.RuneCountInString(c.Text.Body); n > MaxTextBodyLength {
		return fmt.Errorf("text body is %d characters, max is %d", n, MaxTextBodyLength)
	}
	return nil
}

// SendText is a convenience wrapper over SendMessage for the common case.
func (c *Client) SendText(ctx context.Context, to, body string) (*SendMessageResponse, error) {
	return c.SendMessage(ctx, NewSendText(to, body))
}
