package api_gateway

import (
	"mime"
	"net/http"
	"strings"
	"time"
)

// sseContentType is the media type of a Server-Sent Events stream
// (https://html.spec.whatwg.org/multipage/server-sent-events.html).
const sseContentType = "text/event-stream"

// isSSERequest reports whether r asks for a Server-Sent Events stream. Accept
// is the only signal available before the upstream responds, and every SSE
// client sends it (the browser EventSource API always does).
func isSSERequest(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept") {
		for _, part := range strings.Split(v, ",") {
			if mt, _, err := mime.ParseMediaType(strings.TrimSpace(part)); err == nil && mt == sseContentType {
				return true
			}
		}
	}
	return false
}

// isSSEResponse reports whether resp is a Server-Sent Events stream. It backs
// up isSSERequest for clients that omit Accept: by the time the upstream has
// answered, its Content-Type settles the question.
func isSSEResponse(resp *http.Response) bool {
	mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return err == nil && mt == sseContentType
}

// clearWriteDeadline drops the server-wide write deadline for the response in
// flight. Server.WriteTimeout would otherwise cut a long-lived stream mid
// flight, and resolveTimeouts never yields zero for it, so it cannot be
// switched off through configuration.
func clearWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}
