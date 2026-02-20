// Package apierr provides a standardised error response format for the
// QubicDB HTTP API.
//
// Every error response returned by the API uses the same JSON envelope:
//
//	{
//	  "ok":       false,
//	  "error":    "human-readable description",
//	  "code":     "MACHINE_READABLE_CODE",
//	  "status":   400
//	}
//
// This makes error handling predictable for all API consumers — clients can
// branch on the "code" field for programmatic handling and show the "error"
// field to humans.
package apierr

import (
	"encoding/json"
	"net/http"
)

// ---------------------------------------------------------------------------
// Error codes — stable, machine-readable identifiers.
//
// These codes form part of the public API contract. Removing or renaming a
// code is a breaking change; adding new codes is always safe.
// ---------------------------------------------------------------------------

const (
	// General
	CodeBadRequest       = "BAD_REQUEST"
	CodeInvalidJSON      = "INVALID_JSON"
	CodeInvalidContent   = "INVALID_CONTENT"
	CodePayloadTooLarge  = "PAYLOAD_TOO_LARGE"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeNotFound         = "NOT_FOUND"
	CodeInternalError    = "INTERNAL_ERROR"
	CodeUnauthorized     = "UNAUTHORIZED"
	CodeRateLimited      = "RATE_LIMITED"
	CodeConflict         = "CONFLICT"
	CodeMutationDisabled = "MUTATION_DISABLED"

	// Brain / Neuron domain
	CodeIndexIDRequired  = "INDEX_ID_REQUIRED"
	CodeNeuronIDRequired = "NEURON_ID_REQUIRED"
	CodeNeuronNotFound   = "NEURON_NOT_FOUND"
	CodeQueryRequired    = "QUERY_REQUIRED"
	CodeUUIDRequired     = "UUID_REQUIRED"

	// Registry domain
	CodeUUIDNotRegistered = "UUID_NOT_REGISTERED"
	CodeUUIDNotFound      = "UUID_NOT_FOUND"
	CodeUUIDConflict      = "UUID_CONFLICT"
)

// ---------------------------------------------------------------------------
// Response type
// ---------------------------------------------------------------------------

// Response is the standard error envelope returned to API clients.
type Response struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error"`
	Code   string `json:"code"`
	Status int    `json:"status"`
}

// ---------------------------------------------------------------------------
// Writer helpers
// ---------------------------------------------------------------------------

// Write serialises an error Response and writes it to w with the appropriate
// HTTP status code. Content-Type is always set to application/json.
func Write(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{
		OK:     false,
		Error:  message,
		Code:   code,
		Status: status,
	})
}

// ---------------------------------------------------------------------------
// Convenience shortcuts for the most common error patterns.
// Each function maps to a specific HTTP status + error code pair so that
// handler code stays concise.
// ---------------------------------------------------------------------------

// BadRequest writes a 400 response with the given code and message.
func BadRequest(w http.ResponseWriter, code, msg string) {
	Write(w, http.StatusBadRequest, code, msg)
}

// NotFound writes a 404 response.
func NotFound(w http.ResponseWriter, code, msg string) {
	Write(w, http.StatusNotFound, code, msg)
}

// MethodNotAllowed writes a 405 response.
func MethodNotAllowed(w http.ResponseWriter) {
	Write(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "method not allowed")
}

// Unauthorized writes a 401 response.
func Unauthorized(w http.ResponseWriter, msg string) {
	Write(w, http.StatusUnauthorized, CodeUnauthorized, msg)
}

// TooManyRequests writes a 429 response.
func TooManyRequests(w http.ResponseWriter, msg string) {
	if msg == "" {
		msg = "too many requests"
	}
	Write(w, http.StatusTooManyRequests, CodeRateLimited, msg)
}

// Conflict writes a 409 response.
func Conflict(w http.ResponseWriter, code, msg string) {
	Write(w, http.StatusConflict, code, msg)
}

// Internal writes a 500 response.
func Internal(w http.ResponseWriter, msg string) {
	Write(w, http.StatusInternalServerError, CodeInternalError, msg)
}

// InvalidJSON writes a 400 response for malformed request bodies.
func InvalidJSON(w http.ResponseWriter) {
	BadRequest(w, CodeInvalidJSON, "invalid JSON in request body")
}

// PayloadTooLarge writes a 413 response when body/content exceeds configured bounds.
func PayloadTooLarge(w http.ResponseWriter, msg string) {
	if msg == "" {
		msg = "payload too large"
	}
	Write(w, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, msg)
}

// IndexIDRequired writes a 400 response when X-Index-ID is missing.
func IndexIDRequired(w http.ResponseWriter) {
	BadRequest(w, CodeIndexIDRequired, "X-Index-ID header or index_id query parameter required")
}

// NeuronIDRequired writes a 400 response when a neuron ID is missing.
func NeuronIDRequired(w http.ResponseWriter) {
	BadRequest(w, CodeNeuronIDRequired, "neuron ID required in path")
}

// QueryRequired writes a 400 response when a search query is empty.
func QueryRequired(w http.ResponseWriter) {
	BadRequest(w, CodeQueryRequired, "query parameter required")
}

// UUIDRequired writes a 400 response when a UUID is missing.
func UUIDRequired(w http.ResponseWriter) {
	BadRequest(w, CodeUUIDRequired, "uuid field required")
}
