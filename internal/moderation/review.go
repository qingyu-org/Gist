package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qingyu-org/Gist/internal/settings"
)

// verdictSchema is sent as a strict JSON schema so the provider cannot answer
// with prose, a code fence or a half-finished object. Keeping the shape tiny
// also keeps the answer short, which is what the output budget is sized for.
const verdictSchema = `{
  "type": "object",
  "properties": {
    "allowed": {
      "type": "boolean",
      "description": "true when the gist may be published publicly"
    },
    "reason": {
      "type": "string",
      "description": "one short sentence explaining the decision"
    }
  },
  "required": ["allowed", "reason"],
  "additionalProperties": false
}`

// responsesRequest is the subset of the OpenAI Responses API used here. Only
// that one endpoint is supported on purpose: the structured-output and
// reasoning parameters are not portable to other request shapes.
type responsesRequest struct {
	Model           string           `json:"model"`
	Instructions    string           `json:"instructions,omitempty"`
	Input           string           `json:"input"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningConfig `json:"reasoning,omitempty"`
	Text            *textConfig      `json:"text,omitempty"`
}

type reasoningConfig struct {
	Effort string `json:"effort"`
}

type textConfig struct {
	Format *formatConfig `json:"format"`
}

type formatConfig struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type responsesResponse struct {
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	// OutputText is the convenience field some gateways return.
	OutputText string `json:"output_text"`
	Output     []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type verdict struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// reviewResult is what one provider call produced. It is returned even when the
// call failed so the review log can record the latency and the model used.
type reviewResult struct {
	Allowed   bool
	Reason    string
	Model     string
	LatencyMs int
}

// review asks the provider for a verdict on one document.
func review(instructions, document string) (*reviewResult, error) {
	endpoint := settings.ModerationEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("moderation endpoint is not configured")
	}
	model := settings.ModerationModel()
	if model == "" {
		model = "gpt-4o-mini"
	}

	payload := responsesRequest{
		Model:           model,
		Instructions:    instructions,
		Input:           document,
		MaxOutputTokens: settings.ModerationMaxOutputTokens(),
		Text: &textConfig{Format: &formatConfig{
			Type:   "json_schema",
			Name:   "moderation_verdict",
			Strict: true,
			Schema: json.RawMessage(verdictSchema),
		}},
	}
	if effort := settings.ModerationReasoning(); effort != "" {
		payload.Reasoning = &reasoningConfig{Effort: effort}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), settings.ModerationTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := settings.ModerationAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	started := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		return &reviewResult{Model: model, LatencyMs: latency}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &reviewResult{Model: model, LatencyMs: latency}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &reviewResult{Model: model, LatencyMs: latency},
			fmt.Errorf("moderation endpoint returned %s: %s", resp.Status, truncate(string(raw), 200))
	}

	var parsed responsesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return &reviewResult{Model: model, LatencyMs: latency},
			fmt.Errorf("cannot decode moderation response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return &reviewResult{Model: model, LatencyMs: latency},
			fmt.Errorf("moderation endpoint error: %s", parsed.Error.Message)
	}
	if parsed.IncompleteDetails != nil && parsed.IncompleteDetails.Reason != "" {
		return &reviewResult{Model: model, LatencyMs: latency},
			fmt.Errorf("moderation response was incomplete: %s", parsed.IncompleteDetails.Reason)
	}

	text := outputText(&parsed)
	if text == "" {
		return &reviewResult{Model: model, LatencyMs: latency},
			fmt.Errorf("moderation response contains no text")
	}

	allowed, reason, err := parseVerdict(text)
	result := &reviewResult{Model: model, LatencyMs: latency}
	if err != nil {
		return result, err
	}
	result.Allowed = allowed
	result.Reason = reason
	return result, nil
}

// outputText pulls the assistant text out of a Responses API payload, preferring
// the convenience field and falling back to walking the output items.
func outputText(r *responsesResponse) string {
	if s := strings.TrimSpace(r.OutputText); s != "" {
		return s
	}
	var b strings.Builder
	for _, item := range r.Output {
		if item.Type != "" && item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			if part.Type == "" || part.Type == "output_text" || part.Type == "text" {
				b.WriteString(part.Text)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// parseVerdict decodes the JSON object. Strict schemas should give a bare
// object, but a gateway may still wrap it, so the first {...} is accepted too.
func parseVerdict(content string) (bool, string, error) {
	trimmed := strings.TrimSpace(content)
	var v verdict
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		return v.Allowed, strings.TrimSpace(v.Reason), nil
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return false, "", fmt.Errorf("moderation response contains no JSON object")
	}
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &v); err != nil {
		return false, "", fmt.Errorf("cannot decode moderation verdict: %w", err)
	}
	return v.Allowed, strings.TrimSpace(v.Reason), nil
}
