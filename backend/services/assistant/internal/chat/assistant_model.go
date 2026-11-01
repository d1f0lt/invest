package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type AssistantModel struct {
	endpoint string
	name     string
	key      string
	client   *http.Client
}

func NewAssistantModel(endpoint, name, key string) (*AssistantModel, error) {
	if endpoint == "" && name == "" {
		return nil, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") || name == "" {
		return nil, errors.New("set ASSISTANT_URL to a Chat Completions endpoint and ASSISTANT_MODEL to a model name")
	}
	return &AssistantModel{
		endpoint: endpoint, name: name, key: key,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

type assistantMessage struct {
	Role       string              `json:"role"`
	Content    *string             `json:"content"`
	ToolCalls  []assistantToolCall `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
}

type assistantToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func chatMessage(role, content string) assistantMessage {
	return assistantMessage{Role: role, Content: &content}
}

var assistantTools = json.RawMessage(`[{
	"type":"function",
	"function":{
		"name":"search_feed",
		"description":"Find recent news or active user forecasts in the app for one exact security ticker.",
		"parameters":{
			"type":"object",
			"properties":{
				"ticker":{"type":"string","description":"Exchange ticker or security ID, e.g. SBER"},
				"kind":{"type":"string","enum":["news","forecast"]}
			},
			"required":["ticker","kind"],
			"additionalProperties":false
		}
	}
}]`)

func (m *AssistantModel) complete(ctx context.Context, messages []assistantMessage, allowTools bool) (assistantMessage, error) {
	payload := struct {
		Model      string             `json:"model"`
		Messages   []assistantMessage `json:"messages"`
		MaxTokens  int                `json:"max_tokens"`
		Tools      json.RawMessage    `json:"tools,omitempty"`
		ToolChoice string             `json:"tool_choice,omitempty"`
	}{Model: m.name, Messages: messages, MaxTokens: 1600, Tools: assistantTools, ToolChoice: "none"}
	if allowTools {
		payload.ToolChoice = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return assistantMessage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(body))
	if err != nil {
		return assistantMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.key != "" {
		req.Header.Set("Authorization", "Bearer "+m.key)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return assistantMessage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return assistantMessage{}, fmt.Errorf("model returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message assistantMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return assistantMessage{}, err
	}
	if len(result.Choices) == 0 || result.Choices[0].Message.Role != "assistant" {
		return assistantMessage{}, errors.New("invalid model response")
	}
	return result.Choices[0].Message, nil
}
