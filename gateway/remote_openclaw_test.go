package gateway

import "testing"

func TestExtractChatResponseText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]interface{}
		want    string
	}{
		{
			name:    "direct message",
			payload: map[string]interface{}{"message": "hello"},
			want:    "hello",
		},
		{
			name: "payload text array",
			payload: map[string]interface{}{
				"payloads": []interface{}{
					map[string]interface{}{"text": "hey BOSS 👋"},
				},
			},
			want: "hey BOSS 👋",
		},
		{
			name: "choices message content",
			payload: map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{"content": "from choices"},
					},
				},
			},
			want: "from choices",
		},
		{
			name: "nested output content parts",
			payload: map[string]interface{}{
				"output": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": "line one"},
						map[string]interface{}{"type": "text", "text": "line two"},
					},
				},
			},
			want: "line one\nline two",
		},
		{
			name: "no chat text fields",
			payload: map[string]interface{}{
				"meta": map[string]interface{}{"model": "gpt-5.3-codex"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractChatResponseText(tt.payload)
			if got != tt.want {
				t.Fatalf("extractChatResponseText()=%q want %q", got, tt.want)
			}
		})
	}
}
