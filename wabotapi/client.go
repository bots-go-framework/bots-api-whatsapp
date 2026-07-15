// Package wabotapi is a client for the WhatsApp Cloud API.
//
// https://developers.facebook.com/docs/whatsapp/cloud-api
package wabotapi

import (
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the Graph API host. Overridable via Client.BaseURL so the
	// client can be driven against an httptest server.
	DefaultBaseURL = "https://graph.facebook.com"

	// DefaultGraphVersion is the Graph API version this client targets.
	// v25.0 was released 2026-02-18 and is the latest as of 2026-07-15.
	//
	// Meta deprecates Graph API versions on a rolling ~2 year schedule, so this
	// is a deliberate, visible constant rather than an inline literal. Check
	// https://developers.facebook.com/docs/graph-api/changelog when bumping, and
	// override via Client.GraphVersion to pin a different one.
	DefaultGraphVersion = "v25.0"

	// DefaultTimeout bounds every request. The Graph API has no server-side cap
	// that protects a client without one.
	DefaultTimeout = 30 * time.Second
)

// Client interacts with the WhatsApp Cloud API on behalf of a single phone number.
//
// A Client is safe for concurrent use.
type Client struct {
	// accessToken is unexported deliberately: it is a bearer credential, and an
	// exported/serializable token field is a leak vector (it can reach logs via
	// %+v or a JSON dump).
	accessToken string

	// phoneNumberID identifies the sending WhatsApp Business phone number.
	// It is the {phone-number-id} path segment, not the display phone number.
	phoneNumberID string

	// BaseURL is the Graph API host. Defaults to DefaultBaseURL.
	BaseURL string

	// GraphVersion is the Graph API version segment, e.g. "v21.0".
	// Defaults to DefaultGraphVersion.
	GraphVersion string

	// HTTPClient performs the requests. Injected so a caller can supply a
	// request-scoped or instrumented client.
	HTTPClient *http.Client
}

// NewClient returns a Client with a default HTTP client.
//
// Panics if accessToken or phoneNumberID is blank: both are unrecoverable
// configuration errors, and the family convention is to fail at construction
// rather than at the first request.
func NewClient(accessToken, phoneNumberID string) *Client {
	return NewClientWithHTTPClient(accessToken, phoneNumberID, &http.Client{
		Timeout: DefaultTimeout,
	})
}

// NewClientWithHTTPClient returns a Client using the supplied *http.Client.
//
// The caller owns the client's Timeout; none is imposed here.
func NewClientWithHTTPClient(accessToken, phoneNumberID string, httpClient *http.Client) *Client {
	if strings.TrimSpace(accessToken) == "" {
		panic("accessToken must not be empty")
	}
	if strings.TrimSpace(phoneNumberID) == "" {
		panic("phoneNumberID must not be empty")
	}
	if httpClient == nil {
		panic("httpClient must not be nil")
	}
	return &Client{
		accessToken:   accessToken,
		phoneNumberID: phoneNumberID,
		BaseURL:       DefaultBaseURL,
		GraphVersion:  DefaultGraphVersion,
		HTTPClient:    httpClient,
	}
}

// PhoneNumberID returns the sending phone number ID this client is bound to.
func (c *Client) PhoneNumberID() string {
	return c.phoneNumberID
}

// baseURL returns the configured base URL, falling back to the default so a
// zero-valued field never produces a request to "/".
func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimSuffix(c.BaseURL, "/")
}

// graphVersion returns the configured Graph API version, falling back to the default.
func (c *Client) graphVersion() string {
	if c.GraphVersion == "" {
		return DefaultGraphVersion
	}
	return c.GraphVersion
}
