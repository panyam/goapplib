package goapplib

import (
	"encoding/json"
	"net/http"
)

// HtmxResponse provides helpers for setting HTMX response headers.
type HtmxResponse struct {
	w http.ResponseWriter
}

// NewHtmxResponse creates a new HtmxResponse wrapper.
func NewHtmxResponse(w http.ResponseWriter) *HtmxResponse {
	return &HtmxResponse{w: w}
}

// Trigger sets the HX-Trigger header to trigger a client-side event.
func (h *HtmxResponse) Trigger(event string) *HtmxResponse {
	h.w.Header().Set("HX-Trigger", event)
	return h
}

// TriggerWithData sets HX-Trigger with JSON data.
func (h *HtmxResponse) TriggerWithData(event string, data any) *HtmxResponse {
	payload := map[string]any{event: data}
	jsonData, err := json.Marshal(payload)
	if err == nil {
		h.w.Header().Set("HX-Trigger", string(jsonData))
	}
	return h
}

// TriggerAfterSettle sets HX-Trigger-After-Settle header.
func (h *HtmxResponse) TriggerAfterSettle(event string) *HtmxResponse {
	h.w.Header().Set("HX-Trigger-After-Settle", event)
	return h
}

// TriggerAfterSwap sets HX-Trigger-After-Swap header.
func (h *HtmxResponse) TriggerAfterSwap(event string) *HtmxResponse {
	h.w.Header().Set("HX-Trigger-After-Swap", event)
	return h
}

// Redirect tells HTMX to perform a client-side redirect.
func (h *HtmxResponse) Redirect(url string) *HtmxResponse {
	h.w.Header().Set("HX-Redirect", url)
	return h
}

// Location does a client-side redirect without full page reload.
func (h *HtmxResponse) Location(url string) *HtmxResponse {
	h.w.Header().Set("HX-Location", url)
	return h
}

// LocationWithContext does a redirect with additional context.
func (h *HtmxResponse) LocationWithContext(spec map[string]any) *HtmxResponse {
	jsonData, err := json.Marshal(spec)
	if err == nil {
		h.w.Header().Set("HX-Location", string(jsonData))
	}
	return h
}

// Refresh tells HTMX to do a full page refresh.
func (h *HtmxResponse) Refresh() *HtmxResponse {
	h.w.Header().Set("HX-Refresh", "true")
	return h
}

// PushURL pushes a new URL onto the browser's history stack.
func (h *HtmxResponse) PushURL(url string) *HtmxResponse {
	h.w.Header().Set("HX-Push-Url", url)
	return h
}

// ReplaceURL replaces the current URL in the browser's history.
func (h *HtmxResponse) ReplaceURL(url string) *HtmxResponse {
	h.w.Header().Set("HX-Replace-Url", url)
	return h
}

// Retarget changes the target element for the swap.
func (h *HtmxResponse) Retarget(selector string) *HtmxResponse {
	h.w.Header().Set("HX-Retarget", selector)
	return h
}

// Reswap changes the swap method.
func (h *HtmxResponse) Reswap(style string) *HtmxResponse {
	h.w.Header().Set("HX-Reswap", style)
	return h
}

// Reselect changes the selection from the response.
func (h *HtmxResponse) Reselect(selector string) *HtmxResponse {
	h.w.Header().Set("HX-Reselect", selector)
	return h
}

// StopPolling tells HTMX to stop polling.
func (h *HtmxResponse) StopPolling() *HtmxResponse {
	h.w.WriteHeader(286)
	return h
}

// IsHtmxRequest checks if the request is an HTMX request.
func IsHtmxRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// IsBoostedRequest checks if the request is a boosted link.
func IsBoostedRequest(r *http.Request) bool {
	return r.Header.Get("HX-Boosted") == "true"
}

// HtmxTarget returns the HX-Target header value.
func HtmxTarget(r *http.Request) string {
	return r.Header.Get("HX-Target")
}

// HtmxTrigger returns the HX-Trigger header value.
func HtmxTrigger(r *http.Request) string {
	return r.Header.Get("HX-Trigger")
}

// HtmxCurrentURL returns the HX-Current-URL header value.
func HtmxCurrentURL(r *http.Request) string {
	return r.Header.Get("HX-Current-URL")
}

// HtmxPrompt returns the HX-Prompt header value.
func HtmxPrompt(r *http.Request) string {
	return r.Header.Get("HX-Prompt")
}
