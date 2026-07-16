package wabotapi

// InboundMessageType is the type discriminator for inbound webhook messages.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages
type InboundMessageType string

// Inbound message types received via the webhook.
const (
	// InboundMessageTypeText is a plain text message.
	InboundMessageTypeText = InboundMessageType("text")

	// InboundMessageTypeImage is an inbound image message.
	InboundMessageTypeImage = InboundMessageType("image")

	// InboundMessageTypeLocation is an inbound location message.
	InboundMessageTypeLocation = InboundMessageType("location")

	// InboundMessageTypeInteractive is an inbound interactive callback (button
	// tap or list selection).
	InboundMessageTypeInteractive = InboundMessageType("interactive")
)

// WebhookRequest is the top-level object delivered to the configured webhook
// endpoint by the WhatsApp Cloud API.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages
type WebhookRequest struct {
	// Object is always "whatsapp_business_account".
	Object string `json:"object"`

	// Entry is the list of changed entries. Multiple entries can arrive in one
	// delivery, though Meta's current practice sends one entry at a time.
	Entry []WebhookEntry `json:"entry"`
}

// WebhookEntry is one WABA-level entry in the webhook payload.
type WebhookEntry struct {
	// ID is the WABA (WhatsApp Business Account) ID.
	ID string `json:"id"`

	// Changes is the list of change events in this entry.
	Changes []WebhookChange `json:"changes"`
}

// WebhookChange is one change event inside a WebhookEntry.
type WebhookChange struct {
	// Value holds the change payload.
	Value WebhookValue `json:"value"`

	// Field identifies the subscription field that triggered the notification.
	// For message events the value is "messages".
	Field string `json:"field"`
}

// WebhookValue is the payload of a WebhookChange.
type WebhookValue struct {
	// MessagingProduct is always "whatsapp".
	MessagingProduct string `json:"messaging_product"`

	// Metadata identifies the receiving business phone number.
	Metadata WebhookMetadata `json:"metadata"`

	// Contacts resolves sender identities when the change includes inbound messages.
	Contacts []WebhookContact `json:"contacts,omitempty"`

	// Messages is the list of inbound messages in this change.
	Messages []InboundMessage `json:"messages,omitempty"`
}

// WebhookMetadata identifies the business phone number that received the event.
type WebhookMetadata struct {
	// DisplayPhoneNumber is the human-readable number, e.g. "15550783881".
	DisplayPhoneNumber string `json:"display_phone_number"`

	// PhoneNumberID is the {phone-number-id} used in API calls.
	PhoneNumberID string `json:"phone_number_id"`
}

// WebhookContact resolves an inbound sender's display name.
type WebhookContact struct {
	Profile WebhookContactProfile `json:"profile"`

	// WaID is the sender's WhatsApp ID.
	WaID string `json:"wa_id"`
}

// WebhookContactProfile carries the sender's display name.
type WebhookContactProfile struct {
	Name string `json:"name"`
}

// InboundMessage is one message received from a WhatsApp user.
//
// The Type field determines which payload field is populated.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages
type InboundMessage struct {
	// From is the sender's WhatsApp phone number (wa_id), e.g. "16505551234".
	From string `json:"from"`

	// ID is the wamid of the inbound message.
	ID string `json:"id"`

	// Timestamp is the Unix epoch time of the message, delivered as a string.
	Timestamp string `json:"timestamp"`

	// Type is the message type discriminator. Switch on this field.
	Type InboundMessageType `json:"type"`

	// Location is populated when Type == InboundMessageTypeLocation.
	Location *InboundLocation `json:"location,omitempty"`
}

// InboundLocation is the location object of an inbound location message.
//
// Latitude and Longitude are always present. Name, Address, and URL are
// optional — the sender may share a raw pin (coordinates only) rather than
// a named place.
//
// Coordinates are in decimal degrees.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages/location
type InboundLocation struct {
	// Latitude of the shared location in decimal degrees.
	Latitude float64 `json:"latitude"`

	// Longitude of the shared location in decimal degrees.
	Longitude float64 `json:"longitude"`

	// Name is the optional place name, e.g. "Philz Coffee".
	// Present only when the sender shares a saved or searched place.
	Name string `json:"name,omitempty"`

	// Address is the optional street address, e.g. "101 Forest Ave, Palo Alto, CA 94301".
	// Present only when the sender shares a named place with an address.
	Address string `json:"address,omitempty"`

	// URL is the optional place URL, e.g. "https://philzcoffee.com/".
	// Present only when Meta can associate the place with a URL.
	URL string `json:"url,omitempty"`
}
