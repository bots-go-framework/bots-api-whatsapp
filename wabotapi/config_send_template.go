package wabotapi

import (
	"context"
	"errors"
	"fmt"
)

// Template parameter format errors.
var (
	// ErrNoTemplateName is returned when a template config has a blank name.
	ErrNoTemplateName = errors.New("template name must not be empty")

	// ErrNoTemplateLanguage is returned when a template config has no language code.
	ErrNoTemplateLanguage = errors.New("template language code must not be empty")

	// ErrMixedParameterFormats is returned when a component mixes named and
	// positional parameters. Meta requires a component to pick one format.
	ErrMixedParameterFormats = errors.New("a component must not mix named and positional parameters")
)

// TemplateComponentType identifies a template component.
type TemplateComponentType string

// Template component types.
const (
	TemplateComponentHeader = TemplateComponentType("header")
	TemplateComponentBody   = TemplateComponentType("body")
	TemplateComponentButton = TemplateComponentType("button")
)

// TemplateParameterType identifies a template parameter's value type.
//
// Only TemplateParameterText is supported by this client so far — see the
// package README roadmap. The other values are declared because they appear in
// the component wire format, but their nested payloads are not modelled yet and
// must not be guessed at.
type TemplateParameterType string

// Template parameter types.
const (
	TemplateParameterText = TemplateParameterType("text")
)

var _ Sendable = (*SendTemplateConfig)(nil)

// SendTemplateConfig sends a pre-approved message template.
//
// Templates are the only way to message a recipient outside the 24-hour
// customer-service window: a free-form send there fails with
// ErrCodeReEngagementRequired, whose documented remediation is exactly
// "Send the recipient a template message instead."
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/message-templates
type SendTemplateConfig struct {
	BaseMessage
	Template TemplateBody `json:"template"`
}

// TemplateBody is the template object of a template message.
type TemplateBody struct {
	// Name is the template's registered name.
	Name string `json:"name"`

	// Language selects the template's localisation.
	Language TemplateLanguage `json:"language"`

	// Components supplies values for the template's placeholders. Templates with
	// no placeholders take no components.
	Components []TemplateComponent `json:"components,omitempty"`
}

// TemplateLanguage selects a template's language.
//
// Only Code is modelled: the deprecated `policy` field is absent from Meta's
// current template payloads and is not sent.
type TemplateLanguage struct {
	// Code is the language and locale code, e.g. "en_US".
	Code string `json:"code"`
}

// TemplateComponent supplies parameters for one part of a template.
type TemplateComponent struct {
	Type TemplateComponentType `json:"type"`

	// SubType and Index apply to button components only. They are declared for
	// wire completeness; this client does not yet build button components.
	SubType string `json:"sub_type,omitempty"`
	Index   string `json:"index,omitempty"`

	Parameters []TemplateParameter `json:"parameters,omitempty"`
}

// TemplateParameter is a single placeholder value.
type TemplateParameter struct {
	Type TemplateParameterType `json:"type"`

	// ParameterName is set for named-format templates and omitted for positional
	// ones. A component must use one format or the other, never both.
	ParameterName string `json:"parameter_name,omitempty"`

	// Text is the value for a TemplateParameterText parameter.
	Text string `json:"text,omitempty"`
}

// isNamed reports whether this parameter uses the named format.
func (p TemplateParameter) isNamed() bool {
	return p.ParameterName != ""
}

// NewSendTemplate builds a template message config.
//
// languageCode is a language and locale code such as "en_US". A template with no
// placeholders needs nothing further; otherwise add parameters via
// WithBodyParams (positional) or WithNamedBodyParams (named).
func NewSendTemplate(to, name, languageCode string) *SendTemplateConfig {
	return &SendTemplateConfig{
		BaseMessage: newBaseMessage(to, MessageTypeTemplate),
		Template: TemplateBody{
			Name:     name,
			Language: TemplateLanguage{Code: languageCode},
		},
	}
}

// WithBodyParams adds positional body parameters, in order.
//
// Positional placeholders are {{1}}, {{2}} and so on, numbered from 1, and
// values must be supplied in the order the placeholders appear. Positional is
// the default format for a template that did not declare one at creation.
func (c *SendTemplateConfig) WithBodyParams(values ...string) *SendTemplateConfig {
	params := make([]TemplateParameter, 0, len(values))
	for _, v := range values {
		params = append(params, TemplateParameter{Type: TemplateParameterText, Text: v})
	}
	return c.withComponent(TemplateComponentBody, params)
}

// NamedParam is one named template parameter.
//
// Named parameters are passed as an ordered slice rather than a map so the
// emitted JSON is deterministic, which keeps payloads diffable and testable.
type NamedParam struct {
	Name  string
	Value string
}

// WithNamedBodyParams adds named body parameters.
//
// Named placeholders are {{first_name}} and so on. A template must have been
// created with the named format to accept these.
func (c *SendTemplateConfig) WithNamedBodyParams(params ...NamedParam) *SendTemplateConfig {
	ps := make([]TemplateParameter, 0, len(params))
	for _, p := range params {
		ps = append(ps, TemplateParameter{
			Type:          TemplateParameterText,
			ParameterName: p.Name,
			Text:          p.Value,
		})
	}
	return c.withComponent(TemplateComponentBody, ps)
}

// withComponent appends or replaces the component of the given type.
func (c *SendTemplateConfig) withComponent(t TemplateComponentType, params []TemplateParameter) *SendTemplateConfig {
	for i, comp := range c.Template.Components {
		if comp.Type == t {
			c.Template.Components[i].Parameters = params
			return c
		}
	}
	c.Template.Components = append(c.Template.Components, TemplateComponent{
		Type:       t,
		Parameters: params,
	})
	return c
}

// InReplyTo threads this template as a reply to the given wamid.
func (c *SendTemplateConfig) InReplyTo(messageID string) *SendTemplateConfig {
	c.Context = &MessageContext{MessageID: messageID}
	return c
}

// Validate implements Sendable.
func (c *SendTemplateConfig) Validate() error {
	if err := c.BaseMessage.Validate(); err != nil {
		return err
	}
	if c.Template.Name == "" {
		return ErrNoTemplateName
	}
	if c.Template.Language.Code == "" {
		return ErrNoTemplateLanguage
	}
	for _, comp := range c.Template.Components {
		if err := comp.validate(); err != nil {
			return fmt.Errorf("component %q: %w", comp.Type, err)
		}
	}
	return nil
}

// validate checks one component's parameters.
func (comp TemplateComponent) validate() error {
	var named, positional int
	for _, p := range comp.Parameters {
		if p.isNamed() {
			named++
		} else {
			positional++
		}
	}
	if named > 0 && positional > 0 {
		return ErrMixedParameterFormats
	}
	return nil
}

// SendTemplate is a convenience wrapper for a template with positional body
// parameters, or none at all.
//
// For named parameters, build the config with NewSendTemplate +
// WithNamedBodyParams and pass it to SendMessage.
func (c *Client) SendTemplate(
	ctx context.Context,
	to, name, languageCode string,
	bodyParams ...string,
) (*SendMessageResponse, error) {
	cfg := NewSendTemplate(to, name, languageCode)
	if len(bodyParams) > 0 {
		cfg = cfg.WithBodyParams(bodyParams...)
	}
	return c.SendMessage(ctx, cfg)
}
