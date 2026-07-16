package wabotapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// locationWebhookPayload is a captured location webhook payload from Meta's
// documentation, used to pin the inbound wire contract.
//
// Source: https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages/location
//
// The coordinates are decimal degrees. Name, address, and url are present
// here because the sender shared a named place; they are omitted for a raw pin.
const locationWebhookPayload = `{
	"object": "whatsapp_business_account",
	"entry": [
		{
			"id": "102290129340398",
			"changes": [
				{
					"value": {
						"messaging_product": "whatsapp",
						"metadata": {
							"display_phone_number": "15550783881",
							"phone_number_id": "106540352242922"
						},
						"contacts": [
							{
								"profile": {"name": "Sheena Nelson"},
								"wa_id": "16505551234"
							}
						],
						"messages": [
							{
								"from": "16505551234",
								"id": "wamid.HBgLMTY1MDM4Nzk0MzkVAgASGBQzQUFERjg0NDEzNDdFODU3MUMxMAA=",
								"timestamp": "1744344496",
								"location": {
									"address": "101 Forest Ave, Palo Alto, CA 94301",
									"latitude": 37.44221496582,
									"longitude": -122.16165924072,
									"name": "Philz Coffee",
									"url": "https://philzcoffee.com/"
								},
								"type": "location"
							}
						]
					},
					"field": "messages"
				}
			]
		}
	]
}`

// TestWebhookRequest_LocationMessage pins parsing of an inbound location
// webhook against Meta's documented example payload. Exact field values are
// asserted so any future wire change becomes a test failure.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages/location
func TestWebhookRequest_LocationMessage(t *testing.T) {
	var req WebhookRequest
	require.NoError(t, json.Unmarshal([]byte(locationWebhookPayload), &req))

	assert.Equal(t, "whatsapp_business_account", req.Object)
	require.Len(t, req.Entry, 1)

	entry := req.Entry[0]
	assert.Equal(t, "102290129340398", entry.ID)
	require.Len(t, entry.Changes, 1)

	change := entry.Changes[0]
	assert.Equal(t, "messages", change.Field)

	val := change.Value
	assert.Equal(t, "whatsapp", val.MessagingProduct)
	assert.Equal(t, "15550783881", val.Metadata.DisplayPhoneNumber)
	assert.Equal(t, "106540352242922", val.Metadata.PhoneNumberID)

	require.Len(t, val.Contacts, 1)
	assert.Equal(t, "Sheena Nelson", val.Contacts[0].Profile.Name)
	assert.Equal(t, "16505551234", val.Contacts[0].WaID)

	require.Len(t, val.Messages, 1)
	msg := val.Messages[0]
	assert.Equal(t, "16505551234", msg.From)
	assert.Equal(t, "wamid.HBgLMTY1MDM4Nzk0MzkVAgASGBQzQUFERjg0NDEzNDdFODU3MUMxMAA=", msg.ID)
	assert.Equal(t, "1744344496", msg.Timestamp)
	assert.Equal(t, InboundMessageTypeLocation, msg.Type)

	require.NotNil(t, msg.Location,
		"location object must be populated for type=location messages")

	loc := msg.Location
	assert.InDelta(t, 37.44221496582, loc.Latitude, 1e-10,
		"latitude must parse as a float64 in decimal degrees")
	assert.InDelta(t, -122.16165924072, loc.Longitude, 1e-10,
		"longitude must parse as a float64 in decimal degrees")
	assert.Equal(t, "Philz Coffee", loc.Name)
	assert.Equal(t, "101 Forest Ave, Palo Alto, CA 94301", loc.Address)
	assert.Equal(t, "https://philzcoffee.com/", loc.URL)
}

// TestWebhookRequest_LocationMessage_PinOnly pins a raw-pin location (no name,
// address, or URL). These fields must be absent from the parsed struct, not
// populated with zero values from a missing JSON key.
func TestWebhookRequest_LocationMessage_PinOnly(t *testing.T) {
	const rawPinPayload = `{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "102290129340398",
			"changes": [{
				"value": {
					"messaging_product": "whatsapp",
					"metadata": {
						"display_phone_number": "15550783881",
						"phone_number_id": "106540352242922"
					},
					"messages": [{
						"from": "16505551234",
						"id": "wamid.RAWPIN",
						"timestamp": "1744344496",
						"type": "location",
						"location": {
							"latitude": 37.442,
							"longitude": -122.161
						}
					}]
				},
				"field": "messages"
			}]
		}]
	}`

	var req WebhookRequest
	require.NoError(t, json.Unmarshal([]byte(rawPinPayload), &req))

	msg := req.Entry[0].Changes[0].Value.Messages[0]
	require.Equal(t, InboundMessageTypeLocation, msg.Type)

	loc := msg.Location
	require.NotNil(t, loc)
	assert.InDelta(t, 37.442, loc.Latitude, 1e-10)
	assert.InDelta(t, -122.161, loc.Longitude, 1e-10)
	assert.Empty(t, loc.Name, "a raw pin has no name")
	assert.Empty(t, loc.Address, "a raw pin has no address")
	assert.Empty(t, loc.URL, "a raw pin has no url")
}

// TestWebhookRequest_NonLocationMessage pins that Location is nil for a
// non-location message, so callers can type-switch cleanly.
func TestWebhookRequest_NonLocationMessage(t *testing.T) {
	const textPayload = `{
		"object": "whatsapp_business_account",
		"entry": [{
			"id": "102290129340398",
			"changes": [{
				"value": {
					"messaging_product": "whatsapp",
					"metadata": {
						"display_phone_number": "15550783881",
						"phone_number_id": "106540352242922"
					},
					"messages": [{
						"from": "16505551234",
						"id": "wamid.TEXT",
						"timestamp": "1744344496",
						"type": "text"
					}]
				},
				"field": "messages"
			}]
		}]
	}`

	var req WebhookRequest
	require.NoError(t, json.Unmarshal([]byte(textPayload), &req))

	msg := req.Entry[0].Changes[0].Value.Messages[0]
	assert.Equal(t, InboundMessageTypeText, msg.Type)
	assert.Nil(t, msg.Location, "Location must be nil for a non-location message")
}
