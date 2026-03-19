package baseagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func buildOpenAICompatibleChatRequest(modelID, systemPrompt, userMessage string) map[string]interface{} {
	return map[string]interface{}{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
		"temperature": 0.2,
	}
}

func buildOpenAICodexResponsesRequest(modelID, systemPrompt, userMessage string) map[string]interface{} {
	return map[string]interface{}{
		"model":        modelID,
		"store":        false,
		"stream":       true,
		"instructions": systemPrompt,
		"input": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": userMessage,
					},
				},
			},
		},
		"text": map[string]string{
			"verbosity": "medium",
		},
	}
}

func parseOpenAICompatibleChatResponse(body []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode model response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("model response has no choices")
	}
	return extractModelContent(parsed.Choices[0].Message.Content), nil
}

func parseOpenAICodexResponses(body []byte) (string, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", errors.New("model response has no output")
	}

	if looksLikeSSEStream(trimmed) {
		if text, err := parseOpenAICodexSSE(trimmed); err == nil && strings.TrimSpace(text) != "" {
			return text, nil
		}
	}

	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content interface{} `json:"content"`
			Text    string      `json:"text"`
		} `json:"output"`
	}
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		if text, sseErr := parseOpenAICodexSSE(trimmed); sseErr == nil && strings.TrimSpace(text) != "" {
			return text, nil
		}
		return "", fmt.Errorf("decode codex response: %w", err)
	}
	if text := strings.TrimSpace(parsed.OutputText); text != "" {
		return text, nil
	}

	parts := make([]string, 0, len(parsed.Output))
	for _, item := range parsed.Output {
		if text := strings.TrimSpace(item.Text); text != "" {
			parts = append(parts, text)
		}
		if text := extractModelContent(item.Content); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n"), nil
	}

	if text, err := parseOpenAICompatibleChatResponse(trimmed); err == nil {
		return text, nil
	}
	return "", errors.New("model response has no output")
}

func looksLikeSSEStream(body []byte) bool {
	return bytes.HasPrefix(body, []byte("data:")) ||
		bytes.Contains(body, []byte("\nevent:")) ||
		bytes.Contains(body, []byte("\ndata:"))
}

func parseOpenAICodexSSE(body []byte) (string, error) {
	raw := strings.ReplaceAll(string(body), "\r\n", "\n")
	events := strings.Split(raw, "\n\n")

	deltaParts := make([]string, 0, 8)
	completedText := ""

	for _, block := range events {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		dataLines := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(dataLines) == 0 {
			continue
		}

		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var evt map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}

		if t, _ := evt["type"].(string); t == "error" {
			if msg := strings.TrimSpace(extractModelContent(evt)); msg != "" {
				return "", fmt.Errorf("codex stream error: %s", msg)
			}
			return "", errors.New("codex stream error")
		}

		if d, ok := evt["delta"].(string); ok && strings.TrimSpace(d) != "" {
			deltaParts = append(deltaParts, d)
		}
		if text, ok := evt["output_text"].(string); ok && strings.TrimSpace(text) != "" {
			completedText = strings.TrimSpace(text)
		}
		if text, ok := evt["text"].(string); ok && strings.TrimSpace(text) != "" && completedText == "" {
			completedText = strings.TrimSpace(text)
		}
		if response, ok := evt["response"].(map[string]interface{}); ok {
			if text := strings.TrimSpace(extractModelContent(response)); text != "" {
				completedText = text
			}
		}
		if output, ok := evt["output"]; ok && completedText == "" {
			if text := strings.TrimSpace(extractModelContent(output)); text != "" {
				completedText = text
			}
		}
	}

	if len(deltaParts) > 0 {
		return strings.TrimSpace(strings.Join(deltaParts, "")), nil
	}
	if completedText != "" {
		return completedText, nil
	}
	return "", errors.New("model response has no output")
}

func parseModelError(statusCode int, body []byte) error {
	type apiErrBody struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	var parsedErr apiErrBody
	if err := json.Unmarshal(body, &parsedErr); err != nil {
		return nil
	}

	detail := strings.TrimSpace(parsedErr.Error.Message)
	if detail == "" {
		detail = strings.TrimSpace(parsedErr.Message)
	}
	if detail == "" {
		return nil
	}

	code := strings.TrimSpace(parsedErr.Error.Code)
	if code == "" {
		code = strings.TrimSpace(parsedErr.Code)
	}
	if code != "" {
		return fmt.Errorf("model request failed with status %d: %s (%s)", statusCode, detail, code)
	}
	return fmt.Errorf("model request failed with status %d: %s", statusCode, detail)
}
