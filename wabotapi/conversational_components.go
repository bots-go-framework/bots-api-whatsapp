package wabotapi

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// Conversational-automation limits.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
const (
	// MaxCommands is the maximum number of commands per phone number.
	MaxCommands = 30

	// MaxCommandNameLength caps a command name in characters.
	//
	// The character limit applies to the name only. Emojis are not supported.
	MaxCommandNameLength = 32

	// MaxCommandDescriptionLength caps a command description (hint) in characters.
	MaxCommandDescriptionLength = 256

	// MaxIceBreakers is the maximum number of ice-breaker prompts per phone number.
	MaxIceBreakers = 4

	// MaxIceBreakerLength caps an individual ice-breaker prompt in characters.
	MaxIceBreakerLength = 80
)

// Command is one conversational command configured for a business phone number.
//
// A command appears when a user types a forward slash in the message thread.
// When tapped, it arrives as an ordinary inbound text message with the command
// string in the body — there is no separate callback update type.
//
// Emojis are not supported in either the name or description.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
type Command struct {
	// Name is the command text (without leading slash). Max MaxCommandNameLength chars.
	Name string `json:"command_name"`

	// Description is the hint shown alongside the command. Max MaxCommandDescriptionLength chars.
	Description string `json:"command_description"`
}

// ConversationalAutomation is the conversational_automation configuration for a
// business phone number, as returned by the read endpoint.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
type ConversationalAutomation struct {
	// Commands is the list of configured conversational commands.
	Commands []Command `json:"commands,omitempty"`

	// Prompts (ice breakers) are tappable strings shown the first time a user
	// opens a thread with the business. Max MaxIceBreakers items, each at most
	// MaxIceBreakerLength characters.
	Prompts []string `json:"prompts,omitempty"`
}

// GetConversationalAutomationResponse is the response from the read endpoint.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
type GetConversationalAutomationResponse struct {
	ConversationalAutomation ConversationalAutomation `json:"conversational_automation"`

	// ID is the phone number ID echoed back in the response.
	ID string `json:"id"`
}

// setCommandsRequest is the body sent when updating commands.
type setCommandsRequest struct {
	Commands []Command `json:"commands"`
}

// setPromptsRequest is the body sent when updating ice-breaker prompts.
type setPromptsRequest struct {
	Prompts []string `json:"prompts"`
}

// GetConversationalAutomation reads the conversational_automation configuration
// for the client's phone number.
//
// Endpoint: GET /{phone-number-id}?fields=conversational_automation
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
func (c *Client) GetConversationalAutomation(ctx context.Context) (*GetConversationalAutomationResponse, error) {
	endpoint := c.phoneNumberID + "?fields=conversational_automation"
	raw, err := c.MakeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	var resp GetConversationalAutomationResponse
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode GetConversationalAutomationResponse: %w", err)
	}
	return &resp, nil
}

// SetCommands configures the conversational commands for the client's phone number.
//
// Passing an empty slice clears all commands. The operation replaces the full
// command list — it is not additive.
//
// Endpoint: POST /{phone-number-id}/conversational_automation
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
func (c *Client) SetCommands(ctx context.Context, commands []Command) error {
	if err := validateCommands(commands); err != nil {
		return err
	}
	endpoint := c.phoneNumberID + "/conversational_automation"
	_, err := c.MakeRequest(ctx, "POST", endpoint, setCommandsRequest{Commands: commands})
	return err
}

// SetIceBreakers configures the ice-breaker prompts for the client's phone number.
//
// Passing an empty slice clears all prompts. The operation replaces the full
// list — it is not additive.
//
// Endpoint: POST /{phone-number-id}/conversational_automation
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/conversational-components
func (c *Client) SetIceBreakers(ctx context.Context, prompts []string) error {
	if err := validateIceBreakers(prompts); err != nil {
		return err
	}
	endpoint := c.phoneNumberID + "/conversational_automation"
	_, err := c.MakeRequest(ctx, "POST", endpoint, setPromptsRequest{Prompts: prompts})
	return err
}

// validateCommands checks that commands satisfy the documented constraints.
func validateCommands(commands []Command) error {
	if len(commands) > MaxCommands {
		return fmt.Errorf("commands: at most %d allowed, got %d", MaxCommands, len(commands))
	}
	for i, cmd := range commands {
		if cmd.Name == "" {
			return fmt.Errorf("commands[%d]: command_name must not be empty", i)
		}
		if n := utf8.RuneCountInString(cmd.Name); n > MaxCommandNameLength {
			return fmt.Errorf("commands[%d]: command_name is %d characters, max is %d", i, n, MaxCommandNameLength)
		}
		if n := utf8.RuneCountInString(cmd.Description); n > MaxCommandDescriptionLength {
			return fmt.Errorf("commands[%d]: command_description is %d characters, max is %d", i, n, MaxCommandDescriptionLength)
		}
	}
	return nil
}

// validateIceBreakers checks that prompts satisfy the documented constraints.
func validateIceBreakers(prompts []string) error {
	if len(prompts) > MaxIceBreakers {
		return fmt.Errorf("ice breakers: at most %d allowed, got %d", MaxIceBreakers, len(prompts))
	}
	for i, p := range prompts {
		if p == "" {
			return fmt.Errorf("ice_breakers[%d]: prompt must not be empty", i)
		}
		if n := utf8.RuneCountInString(p); n > MaxIceBreakerLength {
			return fmt.Errorf("ice_breakers[%d]: prompt is %d characters, max is %d", i, n, MaxIceBreakerLength)
		}
	}
	return nil
}
