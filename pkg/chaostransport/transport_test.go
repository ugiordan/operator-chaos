package chaostransport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRoundTripper struct {
	called   bool
	response *http.Response
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.called = true
	if f.response != nil {
		return f.response, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestChaosTransport_Passthrough_NilConfig(t *testing.T) {
	ct := NewChaosTransport(nil)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called {
		t.Fatal("inner RoundTripper should be called when config is nil")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_Passthrough_InactiveConfig(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "should not fire"},
	})
	fc.Deactivate()

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called {
		t.Fatal("inner should be called when faults are inactive")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_GetFaultInjection(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "chaos get"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called {
		t.Fatal("inner should NOT be called when fault is injected")
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for GET fault, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_PatchFaultReturnsConflict(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpPatch: {ErrorRate: 1.0, Error: "chaos patch"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("PATCH", "http://localhost/api/v1/pods/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for PATCH fault, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_PostFaultReturnsTooManyRequests(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpCreate: {ErrorRate: 1.0, Error: "chaos create"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("POST", "http://localhost/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for POST/Create fault, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_DeleteFaultReturnsForbidden(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpDelete: {ErrorRate: 1.0, Error: "chaos delete"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("DELETE", "http://localhost/api/v1/pods/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for DELETE fault, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_PutFaultReturnsConflict(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpUpdate: {ErrorRate: 1.0, Error: "chaos update"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("PUT", "http://localhost/api/v1/pods/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for PUT/Update fault, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_ChaosConfigExcluded(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "chaos get"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("GET", "http://localhost/api/v1/namespaces/default/configmaps/chaos-config", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called {
		t.Fatal("chaos-config reads should bypass fault injection")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for excluded chaos-config, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_NonChaosConfigNotExcluded(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "chaos get"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("GET", "http://localhost/api/v1/namespaces/default/configmaps/other-config", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called {
		t.Fatal("non chaos-config GET should be fault-injected")
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_UpdateFaultConfigAtomically(t *testing.T) {
	ct := NewChaosTransport(nil)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	resp, _ := rt.RoundTrip(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected passthrough with nil config, got %d", resp.StatusCode)
	}

	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "now active"},
	})
	ct.UpdateFaultConfig(fc)

	inner2 := &fakeRoundTripper{}
	rt2 := ct.WrapTransport(inner2)
	resp2, _ := rt2.RoundTrip(req)
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 after config update, got %d", resp2.StatusCode)
	}
}

func TestChaosTransport_ResponseBodyIsNoBody(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "test"},
	})

	ct := NewChaosTransport(fc)
	rt := ct.WrapTransport(&fakeRoundTripper{})

	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	resp, _ := rt.RoundTrip(req)

	if resp.Body != http.NoBody {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected NoBody, got: %s", string(body))
	}
}

func TestChaosTransport_UnmatchedOperationPassesThrough(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpCreate: {ErrorRate: 1.0, Error: "create only"},
	})

	ct := NewChaosTransport(fc)
	inner := &fakeRoundTripper{}
	rt := ct.WrapTransport(inner)

	req := httptest.NewRequest("GET", "http://localhost/api/v1/pods", nil)
	resp, _ := rt.RoundTrip(req)
	if !inner.called {
		t.Fatal("GET should pass through when only Create fault is configured")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestChaosTransport_UnknownHTTPMethodDefaultsToGet(t *testing.T) {
	op := httpMethodToOperation("OPTIONS")
	if op != OpGet {
		t.Fatalf("expected OpGet for unknown method, got %s", op)
	}
}

func TestChaosTransport_HTTPMethodMapping(t *testing.T) {
	tests := []struct {
		method string
		op     Operation
	}{
		{"GET", OpGet},
		{"get", OpGet},
		{"PUT", OpUpdate},
		{"POST", OpCreate},
		{"PATCH", OpPatch},
		{"DELETE", OpDelete},
		{"HEAD", OpGet},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := httpMethodToOperation(tt.method)
			if got != tt.op {
				t.Fatalf("httpMethodToOperation(%q) = %s, want %s", tt.method, got, tt.op)
			}
		})
	}
}

func TestChaosTransport_StatusCodeMapping(t *testing.T) {
	tests := []struct {
		op     Operation
		status int
	}{
		{OpGet, http.StatusServiceUnavailable},
		{OpList, http.StatusServiceUnavailable},
		{OpCreate, http.StatusTooManyRequests},
		{OpUpdate, http.StatusConflict},
		{OpPatch, http.StatusConflict},
		{OpDelete, http.StatusForbidden},
		{Operation("unknown"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			got := mapChaosErrorToHTTPStatus(tt.op)
			if got != tt.status {
				t.Fatalf("mapChaosErrorToHTTPStatus(%s) = %d, want %d", tt.op, got, tt.status)
			}
		})
	}
}
