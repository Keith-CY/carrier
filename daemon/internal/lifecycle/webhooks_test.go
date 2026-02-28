package lifecycle

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type webhookRoundTripper struct {
	calls  int
	status []int
}

func (rt *webhookRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	rt.calls++
	code := http.StatusOK
	if len(rt.status) >= rt.calls {
		code = rt.status[rt.calls-1]
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func TestWebhookManagerFiresEvent(t *testing.T) {
	rt := &webhookRoundTripper{status: []int{http.StatusOK}}
	mgr := NewWebhookManager("http://example.test/webhook", []string{WebhookEventAgentStarted})
	mgr.client = &http.Client{Transport: rt}
	mgr.sleep = func(_ time.Duration) {}
	if err := mgr.FireEvent(WebhookEvent{Type: WebhookEventAgentStarted, AgentID: "openclaw"}); err != nil {
		t.Fatalf("FireEvent error: %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("calls = %d, want 1", rt.calls)
	}
}

func TestWebhookManagerRetries(t *testing.T) {
	rt := &webhookRoundTripper{status: []int{500, 500, 200}}
	mgr := NewWebhookManager("http://example.test/webhook", []string{WebhookEventAgentStarted})
	mgr.client = &http.Client{Transport: rt}
	mgr.sleep = func(_ time.Duration) {}
	if err := mgr.FireEvent(WebhookEvent{Type: WebhookEventAgentStarted, AgentID: "openclaw"}); err != nil {
		t.Fatalf("FireEvent error: %v", err)
	}
	if rt.calls != 3 {
		t.Fatalf("calls = %d, want 3", rt.calls)
	}
}
