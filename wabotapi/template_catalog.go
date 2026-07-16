package wabotapi

import (
	"context"
	"encoding/json"
	"fmt"
)

// TemplateStatus is the approval/lifecycle status of a message template.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/message-template-api
type TemplateStatus string

// Template statuses as documented by Meta.
const (
	TemplateStatusApproved        = TemplateStatus("APPROVED")
	TemplateStatusPending         = TemplateStatus("PENDING")
	TemplateStatusRejected        = TemplateStatus("REJECTED")
	TemplateStatusArchived        = TemplateStatus("ARCHIVED")
	TemplateStatusDisabled        = TemplateStatus("DISABLED")
	TemplateStatusDeleted         = TemplateStatus("DELETED")
	TemplateStatusPaused          = TemplateStatus("PAUSED")
	TemplateStatusInAppeal        = TemplateStatus("IN_APPEAL")
	TemplateStatusLimitExceeded   = TemplateStatus("LIMIT_EXCEEDED")
	TemplateStatusPendingDeletion = TemplateStatus("PENDING_DELETION")
)

// TemplateCategory is the business-use classification of a template.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/message-template-api
type TemplateCategory string

// Template categories as documented by Meta.
const (
	TemplateCategoryAuthentication = TemplateCategory("AUTHENTICATION")
	TemplateCategoryMarketing      = TemplateCategory("MARKETING")
	TemplateCategoryUtility        = TemplateCategory("UTILITY")
	TemplateCategoryFreeService    = TemplateCategory("FREE_SERVICE")
)

// CatalogComponentType is the type of a template component as returned by the
// template listing endpoint (uppercase, e.g. "HEADER", "BODY").
//
// This is distinct from TemplateComponentType, which is used for the outbound
// send-template wire format and uses lowercase values.
type CatalogComponentType string

// Catalog component types returned by the listing endpoint.
const (
	CatalogComponentTypeHeader           = CatalogComponentType("HEADER")
	CatalogComponentTypeBody             = CatalogComponentType("BODY")
	CatalogComponentTypeFooter           = CatalogComponentType("FOOTER")
	CatalogComponentTypeButtons          = CatalogComponentType("BUTTONS")
	CatalogComponentTypeCarousel         = CatalogComponentType("CAROUSEL")
	CatalogComponentTypeLimitedTimeOffer = CatalogComponentType("LIMITED_TIME_OFFER")
)

// CatalogComponentFormat is the media format of a HEADER component in the
// template listing response.
type CatalogComponentFormat string

// Header component formats returned by the listing endpoint.
const (
	CatalogComponentFormatText     = CatalogComponentFormat("TEXT")
	CatalogComponentFormatImage    = CatalogComponentFormat("IMAGE")
	CatalogComponentFormatVideo    = CatalogComponentFormat("VIDEO")
	CatalogComponentFormatDocument = CatalogComponentFormat("DOCUMENT")
	CatalogComponentFormatLocation = CatalogComponentFormat("LOCATION")
)

// MessageTemplateComponent is one component of a template as returned by the
// listing endpoint. Only the documented fields are modelled.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/message-template-api
type MessageTemplateComponent struct {
	// Type identifies the component role (HEADER, BODY, FOOTER, BUTTONS, …).
	Type CatalogComponentType `json:"type"`

	// Text is the component text, present for TEXT-format HEADER and BODY/FOOTER.
	Text string `json:"text,omitempty"`

	// Format is the media format of a HEADER component.
	Format CatalogComponentFormat `json:"format,omitempty"`
}

// TemplateQualityScore carries Meta's quality assessment for a template.
type TemplateQualityScore struct {
	// Score is "GREEN", "YELLOW", "RED", or "UNKNOWN".
	Score string `json:"score"`
}

// MessageTemplate is one template as returned by the listing endpoint.
//
// Only fields documented on the endpoint reference page are modelled. Fields
// not documented there (e.g. rejected_reason, allow_category_change) are
// deliberately absent — their wire shapes were not verified.
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/message-template-api
type MessageTemplate struct {
	// ID is the template's Graph API node ID.
	ID string `json:"id"`

	// Name is the template's registered name, e.g. "order_confirmation".
	Name string `json:"name"`

	// Language is the locale code, e.g. "en_US".
	Language string `json:"language"`

	// Status is the approval lifecycle state.
	Status TemplateStatus `json:"status"`

	// Category classifies the template's business use.
	Category TemplateCategory `json:"category"`

	// Components is the list of template components (header, body, footer, buttons).
	Components []MessageTemplateComponent `json:"components,omitempty"`

	// QualityScore is Meta's quality assessment. May be absent on older templates.
	QualityScore *TemplateQualityScore `json:"quality_score,omitempty"`

	// CreatedTimestamp is the Unix epoch time at which the template was created.
	CreatedTimestamp int64 `json:"created_timestamp,omitempty"`

	// LastUpdatedTime is the Unix epoch time of the template's last modification.
	LastUpdatedTime int64 `json:"last_updated_time,omitempty"`
}

// ListTemplatesResponse is the paginated response from the template listing endpoint.
type ListTemplatesResponse struct {
	// Data is the list of templates on this page.
	Data []MessageTemplate `json:"data"`

	// Paging carries cursor-based pagination fields.
	Paging *TemplateListPaging `json:"paging,omitempty"`
}

// TemplateListPaging holds cursor fields for paginating through a template list.
type TemplateListPaging struct {
	Cursors *TemplateListCursors `json:"cursors,omitempty"`

	// Next is the full URL to the next page, when present.
	Next string `json:"next,omitempty"`

	// Previous is the full URL to the previous page, when present.
	Previous string `json:"previous,omitempty"`
}

// TemplateListCursors holds before/after cursor opaque strings.
type TemplateListCursors struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// ListTemplates lists message templates for the given WhatsApp Business Account.
//
// wabaID is the WABA (WhatsApp Business Account) ID — distinct from the phone
// number ID the Client is bound to. Template management is a WABA-level
// operation, not a phone-number-level one.
//
// limit controls the page size (0 = server default). Pass the cursor value from
// ListTemplatesResponse.Paging to retrieve subsequent pages.
//
// Endpoint: GET /{waba-id}/message_templates
//
// https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/message-template-api
func (c *Client) ListTemplates(ctx context.Context, wabaID string, limit int, after string) (*ListTemplatesResponse, error) {
	if wabaID == "" {
		return nil, fmt.Errorf("wabaID must not be empty")
	}

	endpoint := wabaID + "/message_templates"
	sep := "?"
	if limit > 0 {
		endpoint += fmt.Sprintf("%slimit=%d", sep, limit)
		sep = "&"
	}
	if after != "" {
		endpoint += fmt.Sprintf("%safter=%s", sep, after)
	}

	raw, err := c.MakeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	var resp ListTemplatesResponse
	if err = json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode ListTemplatesResponse: %w", err)
	}
	return &resp, nil
}
