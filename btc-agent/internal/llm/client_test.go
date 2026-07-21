package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponseContentParsesUsage(t *testing.T) {
	content, usage, err := responseContent([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`))
	if err != nil || content == "" || !usage.Available || usage.PromptTokens != 12 || usage.CompletionTokens != 7 || usage.TotalTokens != 19 {
		t.Fatalf("content=%q usage=%+v err=%v", content, usage, err)
	}
}

func TestResponseContentMissingUsageIsUnknown(t *testing.T) {
	_, usage, err := responseContent([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	if err != nil || usage.Available || usage.TotalTokens != 0 {
		t.Fatalf("missing usage must remain unknown: %+v err=%v", usage, err)
	}
}

func TestSSEContentParsesUsage(t *testing.T) {
	data := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n" +
		"data: [DONE]\n")
	content, usage := sseContent(data)
	if content != "hello" || !usage.Available || usage.TotalTokens != 5 {
		t.Fatalf("content=%q usage=%+v", content, usage)
	}
}

func TestChatJSONObserverRecordsOnceWithoutPrompt(t *testing.T) {
	var observed []CallResult
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"ok\"}"}}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, APIKey: "test-key", Model: "test-model", Purpose: "operator_decision", TriggerSource: "test", TriggerReason: "unit", Observer: func(r CallResult) { observed = append(observed, r) }})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Answer string `json:"answer"`
	}
	if err := client.ChatJSON(context.Background(), "sensitive prompt", &out); err != nil {
		t.Fatal(err)
	}
	if out.Answer != "ok" || len(observed) != 1 || observed[0].Usage.TotalTokens != 13 || observed[0].Purpose != "operator_decision" {
		t.Fatalf("out=%+v observed=%+v", out, observed)
	}
	b, _ := json.Marshal(observed[0])
	if strings.Contains(string(b), "sensitive prompt") || strings.Contains(string(b), "test-key") {
		t.Fatalf("observer leaked prompt or key: %s", b)
	}
}

func TestHTTPErrorDoesNotReturnProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"credential-like-private-detail"}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ChatText(context.Background(), "prompt")
	if err == nil || strings.Contains(err.Error(), "credential-like-private-detail") || err.Error() != "llm http 401" {
		t.Fatalf("unsafe HTTP error: %v", err)
	}
}

func TestChatJSONParseFailureObservedOnce(t *testing.T) {
	var observed []CallResult
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"}}]}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, APIKey: "key", Observer: func(r CallResult) { observed = append(observed, r) }})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := client.ChatJSON(context.Background(), "prompt", &out); err == nil {
		t.Fatal("expected JSON parse error")
	}
	if len(observed) != 1 || observed[0].Status != "error" || observed[0].ErrorClass != "json_parse" {
		t.Fatalf("observed=%+v", observed)
	}
}
