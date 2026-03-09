package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func decodeMemoryRequestBody(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func TestHandleMemoryListAndSearch(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"GET /api/v2/memory": func(w http.ResponseWriter, r *http.Request) {
			if got := strings.TrimSpace(r.URL.Query().Get("subject")); got != "agent-a" {
				t.Fatalf("subject=%q want agent-a", got)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"entries": []map[string]interface{}{
					{"id": "public.team.v1", "type": "public"},
					{"id": "agent-a.private.v1", "type": "per_agent"},
				},
				"attachments": []map[string]interface{}{
					{"agent_id": "agent-a", "memory_id": "public.team.v1"},
				},
				"grants": []map[string]interface{}{
					{"id": "grant-1", "subject": "agent-a", "scope": "shared:profile"},
				},
				"audit": []map[string]interface{}{},
			})
		},
		"POST /api/v2/memory/search": func(w http.ResponseWriter, r *http.Request) {
			payload := decodeMemoryRequestBody(t, r)
			if got := strings.TrimSpace(anyToString(payload["subject"])); got != "agent-a" {
				t.Fatalf("search subject=%q want agent-a", got)
			}
			if got := strings.TrimSpace(anyToString(payload["query"])); got != "fusion" {
				t.Fatalf("search query=%q want fusion", got)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "rec-1", "scope": "agent:agent-a", "score": 0.97, "snippet": "fusion memory"},
				},
			})
		},
	})

	listRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/memory?subject=agent-a", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	listPayload := decodeJSONMap(t, listRec)
	entries, _ := listPayload["entries"].([]interface{})
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2 payload=%v", len(entries), listPayload)
	}

	searchRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/memory/search", `{"subject":"agent-a","query":"fusion"}`)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", searchRec.Code, searchRec.Body.String())
	}
	searchPayload := decodeJSONMap(t, searchRec)
	results, _ := searchPayload["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results=%d want 1 payload=%v", len(results), searchPayload)
	}
}

func TestHandleMemoryInstanceActions(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v2/memory/instance/attach": func(w http.ResponseWriter, r *http.Request) {
			payload := decodeMemoryRequestBody(t, r)
			if got := strings.TrimSpace(anyToString(payload["instanceId"])); got != "picoclaw-main" {
				t.Fatalf("instanceId=%q want picoclaw-main", got)
			}
			if got := strings.TrimSpace(anyToString(payload["scope"])); got != "shared:profile" {
				t.Fatalf("scope=%q want shared:profile", got)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"status": "attached"})
		},
		"POST /api/v2/memory/instance/detach": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"status": "detached"})
		},
		"POST /api/v2/memory/instance/distill": func(w http.ResponseWriter, r *http.Request) {
			payload := decodeMemoryRequestBody(t, r)
			if got := strings.TrimSpace(anyToString(payload["instanceId"])); got != "picoclaw-main" {
				t.Fatalf("distill instanceId=%q want picoclaw-main", got)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"result": map[string]interface{}{
					"runId":      "distill-1",
					"instanceId": "picoclaw-main",
					"status":     "dry_run",
					"dryRun":     true,
				},
			})
		},
	})

	attachRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/memory/instance/attach", `{"instanceId":"picoclaw-main","scope":"shared:profile"}`)
	if attachRec.Code != http.StatusOK {
		t.Fatalf("attach status=%d body=%s", attachRec.Code, attachRec.Body.String())
	}

	detachRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/memory/instance/detach", `{"instanceId":"picoclaw-main","scope":"shared:profile"}`)
	if detachRec.Code != http.StatusOK {
		t.Fatalf("detach status=%d body=%s", detachRec.Code, detachRec.Body.String())
	}

	distillRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/memory/instance/distill", `{"instanceId":"picoclaw-main","dryRun":true}`)
	if distillRec.Code != http.StatusOK {
		t.Fatalf("distill status=%d body=%s", distillRec.Code, distillRec.Body.String())
	}
	distillPayload := decodeJSONMap(t, distillRec)
	result, _ := distillPayload["result"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(result["runId"])); got != "distill-1" {
		t.Fatalf("runId=%q want distill-1 payload=%v", got, distillPayload)
	}
}
