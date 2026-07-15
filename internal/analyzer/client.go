package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client wraps the Azure OpenAI chat completions API.
type Client struct {
	client     *http.Client
	endpoint   string
	apiKey     string
	apiVersion string
}

func NewClient(apiKey, endpoint string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key must be provided")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("API endpoint must be provided")
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	// Require HTTPS so the API key is never sent over cleartext.
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("endpoint must be a valid https:// URL")
	}

	return &Client{
		// Bound every request so a hung Azure endpoint cannot stall workers.
		client:     &http.Client{Timeout: 60 * time.Second},
		endpoint:   endpoint,
		apiKey:     apiKey,
		apiVersion: "2025-01-01-preview",
	}, nil
}

type chatCompletionRequest struct {
	Model          string                  `json:"model"`
	Messages       []chatCompletionMessage `json:"messages"`
	Temperature    float32                 `json:"temperature,omitempty"`
	MaxTokens      int                     `json:"max_completion_tokens,omitempty"`
	ResponseFormat *responseFormat         `json:"response_format,omitempty"`
}

// responseFormat asks Azure OpenAI to guarantee a valid JSON object in the
// reply. The model will not emit markdown fences.
type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) ChatCompletion(ctx context.Context, model, systemMsg, userMsg string, temperature float32, maxTokens int) (string, error) {
	messages := []chatCompletionMessage{}
	if systemMsg != "" {
		messages = append(messages, chatCompletionMessage{Role: "system", Content: systemMsg})
	}
	if userMsg != "" {
		messages = append(messages, chatCompletionMessage{Role: "user", Content: userMsg})
	}

	reqBody := chatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		// Guarantee a valid JSON object reply
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Path-escape the model name to prevent path traversal / header smuggling
	reqURL := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		c.endpoint, url.PathEscape(model), url.QueryEscape(c.apiVersion))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure OpenAI API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}
