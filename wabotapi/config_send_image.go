package wabotapi

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Image message limits.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/image-messages
const (
	// MaxImageCaptionLength caps the optional caption on an image message.
	MaxImageCaptionLength = 1024
)

// Image send errors.
var (
	// ErrImageNoSource means neither an id nor a link was provided.
	ErrImageNoSource = errors.New("image message requires either an id or a link, not both empty")

	// ErrImageBothSources means both id and link were provided; only one is allowed.
	ErrImageBothSources = errors.New("image message must specify id or link, not both")
)

// ImageObject is the image field of an image message.
//
// Supply exactly one of ID or Link. Meta recommends ID (an uploaded media
// asset) over Link for better performance and reliability.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/image-messages
type ImageObject struct {
	// ID is the media asset ID returned by the Media API upload endpoint.
	// Meta recommends this over Link.
	ID string `json:"id,omitempty"`

	// Link is the publicly accessible URL of an image hosted on your server.
	// Use only when you cannot upload the asset first.
	Link string `json:"link,omitempty"`

	// Caption is an optional caption displayed beneath the image.
	// Maximum MaxImageCaptionLength characters.
	Caption string `json:"caption,omitempty"`
}

var _ Sendable = (*SendImageConfig)(nil)

// SendImageConfig sends an image message.
//
// Supply the image via a media asset ID (recommended) or a public URL.
// An optional caption of up to MaxImageCaptionLength characters may be included.
//
// Supported formats: JPEG, PNG (8-bit, RGB or RGBA, max 5 MB).
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/image-messages
type SendImageConfig struct {
	BaseMessage
	Image ImageObject `json:"image"`
}

// NewSendImageByID builds an image message that references an uploaded media asset.
//
// Sending by ID is Meta's recommended approach: the asset is already on Meta's
// servers, so delivery is faster and more reliable than a public URL fetch.
func NewSendImageByID(to, mediaID string) *SendImageConfig {
	return &SendImageConfig{
		BaseMessage: newBaseMessage(to, MessageTypeImage),
		Image:       ImageObject{ID: mediaID},
	}
}

// NewSendImageByLink builds an image message that references a publicly hosted URL.
//
// Use only when you cannot upload the asset first. Meta recommends the ID form.
func NewSendImageByLink(to, link string) *SendImageConfig {
	return &SendImageConfig{
		BaseMessage: newBaseMessage(to, MessageTypeImage),
		Image:       ImageObject{Link: link},
	}
}

// WithCaption sets the optional image caption and returns the config for chaining.
func (c *SendImageConfig) WithCaption(caption string) *SendImageConfig {
	c.Image.Caption = caption
	return c
}

// InReplyTo threads this image message as a reply to the given wamid.
func (c *SendImageConfig) InReplyTo(messageID string) *SendImageConfig {
	c.Context = &MessageContext{MessageID: messageID}
	return c
}

// Validate implements Sendable.
func (c *SendImageConfig) Validate() error {
	if err := c.BaseMessage.Validate(); err != nil {
		return err
	}
	if c.Image.ID == "" && c.Image.Link == "" {
		return ErrImageNoSource
	}
	if c.Image.ID != "" && c.Image.Link != "" {
		return ErrImageBothSources
	}
	if n := utf8.RuneCountInString(c.Image.Caption); n > MaxImageCaptionLength {
		return fmt.Errorf("image caption is %d characters, max is %d", n, MaxImageCaptionLength)
	}
	return nil
}

// SendImageByID is a convenience wrapper that sends an image by media asset ID.
func (c *Client) SendImageByID(ctx context.Context, to, mediaID string) (*SendMessageResponse, error) {
	return c.SendMessage(ctx, NewSendImageByID(to, mediaID))
}

// SendImageByLink is a convenience wrapper that sends an image by public URL.
func (c *Client) SendImageByLink(ctx context.Context, to, link string) (*SendMessageResponse, error) {
	return c.SendMessage(ctx, NewSendImageByLink(to, link))
}
