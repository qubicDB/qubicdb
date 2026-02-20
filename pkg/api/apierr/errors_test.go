package apierr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// decodeResponse reads an httptest.ResponseRecorder into a Response struct.
func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return resp
}

// ---------------------------------------------------------------------------
// Write
// ---------------------------------------------------------------------------

func TestWrite_SetsContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, http.StatusBadRequest, CodeBadRequest, "test")

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}

func TestWrite_SetsStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, http.StatusNotFound, CodeNotFound, "not found")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestWrite_BodyStructure(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, http.StatusBadRequest, CodeInvalidJSON, "bad json")

	resp := decodeResponse(t, rec)

	if resp.OK {
		t.Error("ok field should be false")
	}
	if resp.Error != "bad json" {
		t.Errorf("expected error 'bad json', got %q", resp.Error)
	}
	if resp.Code != CodeInvalidJSON {
		t.Errorf("expected code %q, got %q", CodeInvalidJSON, resp.Code)
	}
	if resp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Status)
	}
}

// ---------------------------------------------------------------------------
// Convenience shortcuts
// ---------------------------------------------------------------------------

func TestBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	BadRequest(rec, CodeBadRequest, "missing field")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Code != CodeBadRequest {
		t.Errorf("expected code %q, got %q", CodeBadRequest, resp.Code)
	}
}

func TestNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, CodeNeuronNotFound, "neuron xyz not found")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Code != CodeNeuronNotFound {
		t.Errorf("expected code %q, got %q", CodeNeuronNotFound, resp.Code)
	}
	if resp.Error != "neuron xyz not found" {
		t.Errorf("unexpected error message: %q", resp.Error)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	MethodNotAllowed(rec)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Code != CodeMethodNotAllowed {
		t.Errorf("expected code %q, got %q", CodeMethodNotAllowed, resp.Code)
	}
}

func TestUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	Unauthorized(rec, "invalid credentials")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Code != CodeUnauthorized {
		t.Errorf("expected code %q, got %q", CodeUnauthorized, resp.Code)
	}
}

func TestConflict(t *testing.T) {
	rec := httptest.NewRecorder()
	Conflict(rec, CodeUUIDConflict, "uuid already exists")

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Code != CodeUUIDConflict {
		t.Errorf("expected code %q, got %q", CodeUUIDConflict, resp.Code)
	}
}

func TestInternal(t *testing.T) {
	rec := httptest.NewRecorder()
	Internal(rec, "something broke")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Code != CodeInternalError {
		t.Errorf("expected code %q, got %q", CodeInternalError, resp.Code)
	}
}

func TestInvalidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	InvalidJSON(rec)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Code != CodeInvalidJSON {
		t.Errorf("expected code %q, got %q", CodeInvalidJSON, resp.Code)
	}
}

func TestIndexIDRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	IndexIDRequired(rec)

	resp := decodeResponse(t, rec)
	if resp.Code != CodeIndexIDRequired {
		t.Errorf("expected code %q, got %q", CodeIndexIDRequired, resp.Code)
	}
}

func TestNeuronIDRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	NeuronIDRequired(rec)

	resp := decodeResponse(t, rec)
	if resp.Code != CodeNeuronIDRequired {
		t.Errorf("expected code %q, got %q", CodeNeuronIDRequired, resp.Code)
	}
}

func TestQueryRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	QueryRequired(rec)

	resp := decodeResponse(t, rec)
	if resp.Code != CodeQueryRequired {
		t.Errorf("expected code %q, got %q", CodeQueryRequired, resp.Code)
	}
}

func TestUUIDRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	UUIDRequired(rec)

	resp := decodeResponse(t, rec)
	if resp.Code != CodeUUIDRequired {
		t.Errorf("expected code %q, got %q", CodeUUIDRequired, resp.Code)
	}
}

// ---------------------------------------------------------------------------
// Verify all codes are unique
// ---------------------------------------------------------------------------

func TestCodesAreUnique(t *testing.T) {
	codes := []string{
		CodeBadRequest, CodeInvalidJSON, CodeMethodNotAllowed,
		CodeNotFound, CodeInternalError, CodeUnauthorized, CodeConflict,
		CodeIndexIDRequired, CodeNeuronIDRequired, CodeNeuronNotFound,
		CodeQueryRequired, CodeUUIDRequired,
		CodeUUIDNotRegistered, CodeUUIDNotFound, CodeUUIDConflict,
	}

	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		if seen[c] {
			t.Errorf("duplicate error code: %q", c)
		}
		seen[c] = true
	}
}
