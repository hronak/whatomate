// Package whatsapp is a client for Meta's WhatsApp Business Cloud API.
//
// It is the only place in Whatomate that speaks to the Graph API: handlers and
// workers call methods here rather than assembling Graph requests themselves.
// The package covers the outbound surface — messages, templates, media, calls,
// flows, catalogs, business profiles and analytics — plus the inbound webhook
// payload types that Meta POSTs back.
//
// # Client
//
// A Client is safe for concurrent use and is normally constructed once per
// process, then shared. [New] takes production defaults; [Option] values
// override them and compose freely:
//
//	client := whatsapp.New(
//		whatsapp.WithLogger(logger),
//		whatsapp.WithBaseURL(cfg.WhatsApp.BaseURL),
//		whatsapp.WithRetry(3, 500*time.Millisecond),
//	)
//
// Per-request credentials travel in an [Account], so one Client serves every
// WhatsApp business account in a multi-tenant deployment:
//
//	msgID, err := client.SendTextMessage(ctx, account, whatsapp.Recipient{Phone: "+15551234567"}, "hello")
//
// Every network method takes a context as its first argument and honors its
// cancellation and deadline.
//
// # Errors
//
// Calls that reach Meta and receive a non-2xx response return a
// [MetaAPIError] carrying the status, Meta's numeric code and any user-facing
// message. Branch on the failure modes worth handling with errors.Is:
//
//	if errors.Is(err, whatsapp.ErrRateLimited) { … }
//
// Transient failures — throttling, 5xx, and anything that never reached Meta —
// are retried automatically with exponential backoff and jitter, honoring a
// Retry-After header when Meta sends one. See [WithRetry].
//
// # Webhooks
//
// Inbound traffic is modeled by [WebhookPayload] and its nested types, which
// mirror Meta's notification schema — messages, statuses, template status
// updates, user preferences and call events. Verify the X-Hub-Signature-256
// header against the account's app secret before trusting a payload.
package whatsapp
