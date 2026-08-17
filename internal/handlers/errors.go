package handlers

import (
	"cmp"
	"errors"
	"fmt"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// The handler layer's error taxonomy.
//
// Handlers previously chose a status code and a client message at each of ~874
// SendErrorEnvelope call sites, which produced two recurring defects: a
// database outage reported as "not found" (because any query error became a
// 404), and internal detail echoed straight back to the caller via
// err.Error(). The types here separate the two audiences — a client-safe
// message and a cause that is only ever logged — and sendError does the
// mapping in one place.
//
// This extends the errEnvelopeSent contract rather than replacing it. There
// are two entry points, matching the two existing conventions:
//
//   - sendError, for handlers, returns what the envelope write returned
//     (normally nil), exactly like the r.SendErrorEnvelope calls it replaces.
//   - failRequest, for helpers, returns errEnvelopeSent so callers can tell
//     "already handled, stop" from success.

// Sentinel kinds, for errors.Is at call sites that need to branch.
var (
	errNotFound     = errors.New("not found")
	errForbidden    = errors.New("forbidden")
	errUnauthorized = errors.New("unauthorized")
	errConflict     = errors.New("conflict")
	errValidation   = errors.New("validation failed")
)

// apiError couples an HTTP status and a client-safe message with the
// underlying cause. The cause is logged; it is never written to the response.
type apiError struct {
	status  int
	message string // safe to send to the client
	kind    error  // one of the sentinels above, for errors.Is
	cause   error  // internal detail, logged only
}

func (e *apiError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Unwrap exposes both the sentinel kind and the cause chain to errors.Is/As.
func (e *apiError) Unwrap() []error {
	if e.cause == nil {
		return []error{e.kind}
	}
	return []error{e.kind, e.cause}
}

// notFound reports that a resource does not exist, or is not visible to this
// organization — the two are deliberately indistinguishable to the caller.
func notFound(label string) error {
	return &apiError{status: fasthttp.StatusNotFound, message: label + " not found", kind: errNotFound}
}

// forbidden reports that the caller is authenticated but not permitted.
func forbidden(message string) error {
	return &apiError{status: fasthttp.StatusForbidden, message: message, kind: errForbidden}
}

// unauthorized reports missing or invalid credentials.
func unauthorized(message string) error {
	return &apiError{status: fasthttp.StatusUnauthorized, message: message, kind: errUnauthorized}
}

// conflict reports a request that clashes with current state (duplicate name,
// already-running campaign, and so on).
func conflict(message string) error {
	return &apiError{status: fasthttp.StatusConflict, message: message, kind: errConflict}
}

// invalidRequest reports a client-correctable problem with the request. The
// message is written by us, so it is safe to return verbatim.
func invalidRequest(message string) error {
	return &apiError{
		status:  fasthttp.StatusBadRequest,
		message: message,
		kind:    errValidation,
	}
}

// invalidRequestf is invalidRequest with formatting.
func invalidRequestf(format string, args ...any) error {
	return invalidRequest(fmt.Sprintf(format, args...))
}

// upstreamError reports that a service we called on the caller's behalf failed.
// The fault is not ours, so it must not be reported as a 500 — 502 tells the
// caller (and any monitoring) that the dependency broke, not this server.
func upstreamError(message string, cause error) error {
	return &apiError{
		status:  fasthttp.StatusBadGateway,
		message: message,
		cause:   cause,
	}
}

// internalError wraps an unexpected failure. The caller sees a generic message;
// cause reaches the log only.
func internalError(message string, cause error) error {
	return &apiError{
		status:  fasthttp.StatusInternalServerError,
		message: message,
		cause:   cause,
	}
}

// fromMetaError turns a pkg/whatsapp failure into an apiError.
//
// Meta's own description of the rejection is surfaced: these handlers are
// account-setup flows, and "Phone number must be verified before registration"
// is precisely what the operator needs to read. That is Meta's account of what
// the caller did wrong, not our internals — error_user_msg first, since it is
// written for humans, then the developer-facing message.
//
// What is *not* surfaced is everything the caller cannot act on: trace IDs,
// our own error wrapping, and any failure where we never got a considered
// answer from Meta at all. Those fall back to a generic message and are logged.
func fromMetaError(fallback string, err error) error {
	status := fasthttp.StatusBadGateway

	var apiErr *whatsapp.MetaAPIError
	if errors.As(err, &apiErr) {
		// A rejection Meta reasoned about is the caller's problem to fix.
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			status = fasthttp.StatusBadRequest
		}
		if msg := cmp.Or(apiErr.Detail.ErrorUserMsg, apiErr.Detail.Message); msg != "" {
			return &apiError{status: status, message: msg, kind: errValidation, cause: err}
		}
	}
	return &apiError{status: status, message: fallback, cause: err}
}

// sendError writes the error envelope for err and returns whatever the
// envelope write returned — normally nil. It is the handler-level form:
//
//	return a.sendError(r, notFound("Campaign"))
//
// matching the contract of the r.SendErrorEnvelope calls it replaces, where a
// handler that has written a response returns nil to fastglue.
func (a *App) sendError(r *fastglue.Request, err error) error {
	return a.writeError(r, err)
}

// failRequest writes the error envelope and returns errEnvelopeSent. It is the
// helper-level form, for functions whose callers must distinguish "already
// handled, stop" from success — see findByIDAndOrg.
func (a *App) failRequest(r *fastglue.Request, err error) error {
	_ = a.writeError(r, err)
	return errEnvelopeSent
}

// writeError maps err onto an HTTP status and writes the envelope.
//
// Anything that is not a recognized apiError is treated as a server fault: the
// caller gets a generic 500 and the detail is logged. gorm.ErrRecordNotFound is
// mapped to 404 as a convenience for handlers that pass a raw query error
// through.
func (a *App) writeError(r *fastglue.Request, err error) error {
	if err == nil {
		return nil
	}
	// Already handled by a helper that wrote its own envelope.
	if errors.Is(err, errEnvelopeSent) {
		return nil
	}

	var apiErr *apiError
	switch {
	case errors.As(err, &apiErr):
	case errors.Is(err, gorm.ErrRecordNotFound):
		apiErr = &apiError{status: fasthttp.StatusNotFound, message: "Resource not found", kind: errNotFound}
	default:
		apiErr = &apiError{
			status:  fasthttp.StatusInternalServerError,
			message: "Internal server error",
			cause:   err,
		}
	}

	// Log the cause for server faults; client errors are self-explanatory and
	// logging them at Error level is what skewed this codebase to 666 Error
	// records against 68 Warn.
	if apiErr.status >= fasthttp.StatusInternalServerError {
		a.Log.Error("Request failed",
			"error", apiErr.cause,
			"message", apiErr.message,
			"method", string(r.RequestCtx.Method()),
			"path", string(r.RequestCtx.Path()),
		)
	}

	return r.SendErrorEnvelope(apiErr.status, apiErr.message, nil, "")
}
