package wabotapi

import (
	"context"
	"encoding/json"
	"fmt"
)

// MessagingProduct is the Graph API product selector. Always "whatsapp" here.
type MessagingProduct string

// MessagingProductWhatsApp is the only valid MessagingProduct for this client.
const MessagingProductWhatsApp = MessagingProduct("whatsapp")

// RecipientType distinguishes individual recipients from groups.
type RecipientType string

// RecipientTypeIndividual addresses a single WhatsApp user.
// Group messaging is not supported by the Cloud API.
const RecipientTypeIndividual = RecipientType("individual")

// MessageType is the outbound message type discriminator.
//
// https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages
type MessageType string

// Outbound message types.
const (
	MessageTypeText        = MessageType("text")
	MessageTypeImage       = MessageType("image")
	MessageTypeAudio       = MessageType("audio")
	MessageTypeDocument    = MessageType("document")
	MessageTypeSticker     = MessageType("sticker")
	MessageTypeVideo       = MessageType("video")
	MessageTypeLocation    = MessageType("location")
	MessageTypeContacts    = MessageType("contacts")
	MessageTypeInteractive = MessageType("interactive")
	MessageTypeTemplate    = MessageType("template")
	MessageTypeReaction    = MessageType("reaction")
)

// BaseMessage carries the fields every outbound message shares.
//
// Embedded by each message config, which then adds its own type-specific field.
// Mirrors the embedded-base-then-extend idiom used throughout bots-api-telegram.
type BaseMessage struct {
	// MessagingProduct must be MessagingProductWhatsApp.
	MessagingProduct MessagingProduct `json:"messaging_product"`

	// RecipientType defaults to individual when omitted.
	RecipientType RecipientType `json:"recipient_type,omitempty"`

	// To is the recipient's WhatsApp ID (wa_id) or a phone number in E.164
	// without the leading "+". Note this is the platform's user identity: a
	// phone number, not a stable integer as on Telegram.
	To string `json:"to"`

	// Type is the message type discriminator.
	Type MessageType `json:"type"`

	// Context threads a reply to a specific inbound message.
	Context *MessageContext `json:"context,omitempty"`
}

// MessageContext marks a message as a reply to a previous one.
type MessageContext struct {
	// MessageID is the wamid of the message being replied to.
	MessageID string `json:"message_id"`
}

// newBaseMessage builds a BaseMessage with the invariant fields set.
func newBaseMessage(to string, msgType MessageType) BaseMessage {
	return BaseMessage{
		MessagingProduct: MessagingProductWhatsApp,
		RecipientType:    RecipientTypeIndividual,
		To:               to,
		Type:             msgType,
	}
}

// Endpoint implements Sendable: all outbound messages POST to the same path.
func (m BaseMessage) Endpoint(phoneNumberID string) string {
	return phoneNumberID + "/messages"
}

// Validate implements Sendable for the fields every message shares.
func (m BaseMessage) Validate() error {
	if m.To == "" {
		return ErrNoRecipient
	}
	if m.MessagingProduct != MessagingProductWhatsApp {
		return fmt.Errorf("messaging_product must be %q, got %q", MessagingProductWhatsApp, m.MessagingProduct)
	}
	if m.Type == "" {
		return fmt.Errorf("message type must not be empty")
	}
	return nil
}

// SendMessageResponse is the result of a successful POST to /{phone-number-id}/messages.
//
// https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#response
type SendMessageResponse struct {
	MessagingProduct MessagingProduct `json:"messaging_product"`

	// Contacts echoes the resolved recipient. The wa_id may differ from the
	// number that was submitted.
	Contacts []ResponseContact `json:"contacts"`

	// Messages carries the accepted message IDs.
	Messages []ResponseMessage `json:"messages"`
}

// ResponseContact is a resolved recipient in a SendMessageResponse.
type ResponseContact struct {
	// Input is the recipient string as submitted.
	Input string `json:"input"`

	// WaID is WhatsApp's canonical ID for that recipient.
	WaID string `json:"wa_id"`
}

// ResponseMessage is an accepted message in a SendMessageResponse.
type ResponseMessage struct {
	// ID is the wamid, e.g. "wamid.HBgLMTY1MDUwNzY1MjAVAgARGBI5QTND...".
	ID string `json:"id"`

	// MessageStatus is present when the message was accepted but held, e.g.
	// "accepted" or "held_for_quality_assessment".
	MessageStatus string `json:"message_status,omitempty"`
}

// MessageID returns the wamid of the first accepted message, or "" if none.
func (r *SendMessageResponse) MessageID() string {
	if len(r.Messages) == 0 {
		return ""
	}
	return r.Messages[0].ID
}

// SendMessage sends a message config and decodes the messages-endpoint response.
//
// Use Send directly when the raw payload is wanted.
func (c *Client) SendMessage(ctx context.Context, cfg Sendable) (*SendMessageResponse, error) {
	raw, err := c.Send(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var resp SendMessageResponse
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode SendMessageResponse: %w", err)
	}
	return &resp, nil
}
