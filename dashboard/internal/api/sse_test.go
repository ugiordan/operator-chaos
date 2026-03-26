package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSSEBroker_ClientReceivesEvent(t *testing.T) {
	broker := NewSSEBroker()
	go broker.Run()
	defer broker.Stop()

	time.Sleep(10 * time.Millisecond)

	done := make(chan string, 1)
	handler := broker.ServeHTTP

	req := httptest.NewRequest("GET", "/api/v1/experiments/live", nil)
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder(), done: done}

	go handler(rec, req)

	time.Sleep(50 * time.Millisecond)

	broker.Broadcast([]byte(`{"name":"test","phase":"Observing"}`))

	select {
	case data := <-done:
		assert.Contains(t, data, `"name":"test"`)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SSE event")
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	done chan string
}

func (f *flushRecorder) Flush() {
	body := f.Body.String()
	if strings.Contains(body, "data:") {
		f.done <- body
	}
}
