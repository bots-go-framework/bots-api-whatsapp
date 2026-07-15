# Go bindings for the WhatsApp Cloud API

[![Go CI](https://github.com/bots-go-framework/bots-api-whatsapp/actions/workflows/ci.yml/badge.svg)](https://github.com/bots-go-framework/bots-api-whatsapp/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bots-go-framework/bots-api-whatsapp)](https://goreportcard.com/report/github.com/bots-go-framework/bots-api-whatsapp)
[![GoDoc](https://pkg.go.dev/badge/github.com/bots-go-framework/bots-api-whatsapp)](https://pkg.go.dev/github.com/bots-go-framework/bots-api-whatsapp)

Go bindings for the [WhatsApp Cloud API](https://developers.facebook.com/docs/whatsapp/cloud-api),
targeting **Graph API v25.0**.

> **Status: early.** The client core (transport, retry, error model) is in place
> and tested. The message-type surface is currently text-only — see
> [Roadmap](#roadmap).

This module targets the **Cloud API** (Meta-hosted). The On-Premises API is not
supported.

<!-- dev-approach:v1 -->
## Our approach to development

We build with our own tooling:

- **[SpecScore](https://specscore.md)** — specify requirements as `SpecScore.md` artifacts
- **[SpecStudio](https://specscore.studio)** — author & manage specs across their lifecycle
- **[inGitDB](https://ingitdb.com)** — store structured data in Git where applicable
- **[DALgo](https://dalgo.io)** — data access layer for Go
- **[cover100.dev](https://cover100.dev)** — drive toward 100% test coverage
- **[DataTug](https://datatug.io)** — query & explore data
<!-- /dev-approach -->

## Packages

| Package | Import path | Description |
|---|---|---|
| `wabotapi` | `github.com/bots-go-framework/bots-api-whatsapp/wabotapi` | Core Cloud API types and HTTP client |

## Installation

```shell
go get github.com/bots-go-framework/bots-api-whatsapp
```

## Usage

```go
package main

import (
	"context"
	"log"

	"github.com/bots-go-framework/bots-api-whatsapp/wabotapi"
)

func main() {
	client := wabotapi.NewClient(accessToken, phoneNumberID)

	resp, err := client.SendText(context.Background(), "16505551234", "Hello!")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("sent:", resp.MessageID())
}
```

### Handling the 24-hour window

WhatsApp only permits free-form messages within 24 hours of the recipient's last
reply. Outside that window, only pre-approved template messages may be sent, and
the API rejects the send with error code `131047`.

This has no Telegram analogue, so it is worth handling explicitly:

```go
resp, err := client.SendText(ctx, to, text)
if apiErr := wabotapi.AsAPIError(err); apiErr != nil {
	switch {
	case apiErr.IsReEngagementRequired():
		// The 24h window has closed — send an approved template instead.
	case apiErr.IsRateLimited():
		// Already retried with backoff; back off further or shed load.
	case apiErr.IsUnreachable():
		// Recipient cannot be reached; do not retry.
	case apiErr.IsAuthError():
		// Token missing, invalid, or expired.
	}
}
```

Callers that would rather not import this package can test the behavior
interfaces directly:

```go
if e, ok := err.(interface{ IsReEngagementRequired() bool }); ok && e.IsReEngagementRequired() {
	// ...
}
```

### Testing against a local server

`Client.BaseURL` is overridable, so the client can be driven against an
`httptest.Server`:

```go
ts := httptest.NewServer(handler)
defer ts.Close()

c := wabotapi.NewClientWithHTTPClient(token, phoneNumberID, ts.Client())
c.BaseURL = ts.URL
```

### Graph API version

`Client.GraphVersion` defaults to `wabotapi.DefaultGraphVersion` and can be
overridden per client. Meta deprecates Graph API versions on a rolling ~2 year
schedule; check the
[changelog](https://developers.facebook.com/docs/graph-api/changelog) before
relying on the default.

## Design notes

This client follows the conventions of its sibling
[bots-api-telegram](https://github.com/bots-go-framework/bots-api-telegram) —
repo skeleton, `<Method>Config` type naming, embedded-base-then-extend, doc
comments ending in the upstream API URL, and `var _ Sendable = (*T)(nil)`
assertions.

It deliberately deviates in a few places, each of which is a known wart in the
older clients rather than a considered convention:

| Deviation | Why |
|---|---|
| `BaseURL` is an overridable field, not a `const` | A `const` endpoint is why the older clients have no `httptest` coverage of their core |
| `ctx` is the first parameter of every API call and reaches `http.NewRequestWithContext` | Telegram's client holds a context for logging only and never propagates it |
| Default `http.Client.Timeout` | The older clients use a bare `&http.Client{}` with no timeout |
| JSON request bodies | `url.Values` encoding is a Telegram Bot API accommodation; the Cloud API takes JSON |
| Retry classifies on `error.code`, with exponential backoff | Telegram's client string-matches a GAE error and has no rate-limit handling. See the note below on why *not* 429/`Retry-After`. |
| Access token is unexported | An exported, serializable token field is a leak vector |
| Errors never echo request URL or body | The older client formats request params into a 401 error, and those reach logs |

### Why not `429` / `Retry-After`

The Cloud API does **not** signal throttling with HTTP 429, and does **not** send
a `Retry-After` header. Meta's error reference mentions neither: rate limits
arrive as **HTTP 400** carrying an error code (`4`, `80007`, `130429`, `131048`,
`131064`).

This matters because several third-party clients — including at least one Go
library — annotate those codes as 429. Building on that assumption yields retry
logic that never fires.

So this client classifies on `error.code` and ignores HTTP status for that
decision. `Retry-After` is still honored *if present*, defensively, in case an
edge or proxy throttles ahead of Meta — but nothing depends on it appearing.

For the same reason, never string-match error text: Meta's documented wording for
`131047` and the wording that actually arrives on the wire differ, and Meta says
code titles "will eventually be deprecated". Branch on the integer.

## Roadmap

Implemented:

- Client core: bearer auth, JSON transport, retry with backoff, typed errors
- `text` messages
- `template` messages — named and positional body parameters

Not yet implemented:

- Template `header` and `button` components, and the `currency` / `date_time` /
  media parameter types. Deliberately omitted: Meta's current docs do not spell
  out their nested shapes, and guessing at a wire format is worse than not
  shipping it.
- Media: `image`, `audio`, `document`, `sticker`, `video` (upload → media ID → send by ID)
- `interactive` messages (buttons, lists)
- `location`, `contacts`, `reaction`
- Inbound webhook payload types, `X-Hub-Signature-256` verification, and the
  `hub.challenge` verification handshake
- Message status callbacks (sent / delivered / read / failed)

## Sending a template

```go
// No placeholders.
resp, err := client.SendTemplate(ctx, to, "hello_world", "en_US")

// Positional placeholders: {{1}}, {{2}} ...
resp, err := client.SendTemplate(ctx, to, "order_confirmation", "en_US",
	"Jessica", "SKBUP2-4CPIG9")

// Named placeholders: {{first_name}} ...
cfg := wabotapi.NewSendTemplate(to, "order_confirmation", "en_US").
	WithNamedBodyParams(
		wabotapi.NamedParam{Name: "first_name", Value: "Jessica"},
		wabotapi.NamedParam{Name: "order_number", Value: "SKBUP2-4CPIG9"},
	)
resp, err := client.SendMessage(ctx, cfg)
```

A template declares its parameter format at creation time and defaults to
positional. A component may not mix the two formats; this client rejects that
locally rather than spending a round trip to earn a `132018` validation error.

## Related

- [bots-fw](https://github.com/bots-go-framework/bots-fw) — the bot framework this
  client is intended to plug into via a `bots-fw-whatsapp` adapter
- [bots-api-telegram](https://github.com/bots-go-framework/bots-api-telegram) — the
  sibling client this one is modelled on
