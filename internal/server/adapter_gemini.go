package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/llaoj/aist/internal/util"
	"github.com/pocketbase/pocketbase/tools/security"
)

// =============================================================================
// 1. Endpoint, auth and model-name rules
// =============================================================================

// geminiUpstreamAdapter translates between OpenAI Chat Completions and Google's
// native Gemini generateContent REST API, so multi-turn tool calls never depend
// on Google's /v1beta/openai translation layer.
//
//  1. endpoint, auth and model-name rules
//  2. OpenAI request  -> Gemini request
//  3. Gemini response -> OpenAI response
type geminiUpstreamAdapter struct{}

const geminiSkipThoughtSignature = "skip_thought_signature_validator"

// BuildRequest posts to {base_url}/models/{model}:generateContent; base_url is
// the native API root, e.g. https://generativelanguage.googleapis.com/v1beta.
func (geminiUpstreamAdapter) BuildRequest(ctx context.Context, baseURL, apiKey string, candidate modelCandidate, openAIBody []byte) (*http.Request, error) {
	body, err := toGeminiRequest(openAIBody, candidate)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/models/" + url.PathEscape(candidate.Model) + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("x-goog-api-key", apiKey)
	}
	return req, nil
}

func (geminiUpstreamAdapter) NormalizeResponse(modelName string, respBody []byte) ([]byte, error) {
	return fromGeminiResponse(modelName, respBody)
}

// omitsGeminiCallIDs mirrors OpenCode's /^gemini(?:-live)?-(\d+)/ rule: Gemini
// 1.x and 2.x reject explicit functionCall/functionResponse IDs, Gemini 3+ and
// non-Gemini model IDs accept them.
func omitsGeminiCallIDs(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	rest, ok := strings.CutPrefix(name, "gemini-live-")
	if !ok {
		if rest, ok = strings.CutPrefix(name, "gemini-"); !ok {
			return false
		}
	}
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	major, err := strconv.Atoi(rest[:digits])
	return err == nil && major < 3
}

// =============================================================================
// 2. OpenAI request -> Gemini request
// =============================================================================

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	CachedContent     json.RawMessage         `json:"cachedContent,omitempty"`
	ServiceTier       json.RawMessage         `json:"serviceTier,omitempty"`
	SafetySettings    json.RawMessage         `json:"safetySettings,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	ParametersJSONSchema json.RawMessage `json:"parametersJsonSchema,omitempty"`
}

type geminiToolConfig struct {
	FunctionCallingConfig geminiFunctionCallingConfig `json:"functionCallingConfig"`
}

type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"topP,omitempty"`
	ThinkingConfig  json.RawMessage `json:"thinkingConfig,omitempty"`
}

// toGeminiRequest converts an OpenAI Chat Completions body into a Gemini
// generateContent body.
func toGeminiRequest(openAIBody []byte, candidate modelCandidate) ([]byte, error) {
	var in openAIInboundRequest
	if err := json.Unmarshal(openAIBody, &in); err != nil {
		return nil, err
	}

	b := newGeminiTurnBuilder(in.Messages, omitsGeminiCallIDs(candidate.Model))
	for _, msg := range in.Messages {
		b.add(msg)
	}
	contents, err := b.contents()
	if err != nil {
		return nil, err
	}

	out := geminiRequest{
		SystemInstruction: b.systemInstruction(),
		Contents:          contents,
		GenerationConfig:  buildGeminiGenerationConfig(in, candidate),
		CachedContent:     candidate.ExtraBody["cachedContent"],
		ServiceTier:       candidate.ExtraBody["serviceTier"],
		SafetySettings:    candidate.ExtraBody["safetySettings"],
	}
	if decls := lowerGeminiTools(in.Tools); len(decls) > 0 {
		out.Tools = []geminiTool{{FunctionDeclarations: decls}}
		out.ToolConfig = lowerGeminiToolConfig(in.ToolChoice)
	}
	return json.Marshal(out)
}

// geminiTurnBuilder turns OpenAI messages into Gemini contents while keeping
// Gemini's turn rules (same as OpenCode):
//   - system messages before the conversation become systemInstruction; later
//     ones become wrapped user text;
//   - all results of one parallel call batch share one functionResponse turn;
//   - user text never joins a functionResponse turn, it starts a new user turn.
type geminiTurnBuilder struct {
	omitCallIDs bool
	callNames   map[string]string // tool_call_id -> function name
	system      []geminiPart
	turns       []geminiContent
}

func newGeminiTurnBuilder(messages []openAIInboundMessage, omitCallIDs bool) *geminiTurnBuilder {
	callNames := make(map[string]string)
	for _, msg := range messages {
		for _, call := range msg.ToolCalls {
			if call.ID != "" && call.Function.Name != "" {
				callNames[call.ID] = call.Function.Name
			}
		}
	}
	return &geminiTurnBuilder{omitCallIDs: omitCallIDs, callNames: callNames}
}

func (b *geminiTurnBuilder) add(msg openAIInboundMessage) {
	switch msg.Role {
	case "system", "developer":
		b.addSystem(extractMessageText(msg.Content))
	case "user":
		text := extractMessageText(msg.Content)
		if text == "" {
			text = "(empty)"
		}
		b.addUserText(text)
	case "assistant":
		b.addModelParts(lowerAssistantParts(msg, b.omitCallIDs))
	case "tool":
		b.addFunctionResponse(b.lowerToolResult(msg))
	}
}

func (b *geminiTurnBuilder) addSystem(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if len(b.turns) == 0 {
		b.system = append(b.system, geminiPart{Text: text})
		return
	}
	b.addUserText("<system-update>\n" + text + "\n</system-update>")
}

func (b *geminiTurnBuilder) addUserText(text string) {
	part := geminiPart{Text: text}
	if last := b.lastTurn("user"); last != nil && !last.hasFunctionResponse() {
		last.Parts = append(last.Parts, part)
		return
	}
	b.turns = append(b.turns, geminiContent{Role: "user", Parts: []geminiPart{part}})
}

func (b *geminiTurnBuilder) addModelParts(parts []geminiPart) {
	if len(parts) == 0 {
		return
	}
	if last := b.lastTurn("model"); last != nil {
		last.Parts = append(last.Parts, parts...)
		return
	}
	b.turns = append(b.turns, geminiContent{Role: "model", Parts: parts})
}

func (b *geminiTurnBuilder) addFunctionResponse(part geminiPart) {
	if last := b.lastTurn("user"); last != nil && last.hasFunctionResponse() {
		last.Parts = append(last.Parts, part)
		return
	}
	b.turns = append(b.turns, geminiContent{Role: "user", Parts: []geminiPart{part}})
}

// lastTurn returns the last turn if it has the given role.
func (b *geminiTurnBuilder) lastTurn(role string) *geminiContent {
	if n := len(b.turns); n > 0 && b.turns[n-1].Role == role {
		return &b.turns[n-1]
	}
	return nil
}

func (b *geminiTurnBuilder) lowerToolResult(msg openAIInboundMessage) geminiPart {
	name := util.FirstNonEmpty(msg.Name, b.callNames[msg.ToolCallID], "unknown_tool")
	return geminiPart{FunctionResponse: &geminiFunctionResponse{
		ID:   geminiCallID(msg.ToolCallID, b.omitCallIDs),
		Name: name,
		Response: map[string]any{
			"name":    name,
			"content": extractMessageText(msg.Content),
		},
	}}
}

// contents returns the turns; a leading model turn gets a user turn in front
// because Gemini requires a function call turn to follow a user turn.
func (b *geminiTurnBuilder) contents() ([]geminiContent, error) {
	if len(b.turns) == 0 {
		return nil, errors.New("request messages produced empty Gemini contents")
	}
	if b.turns[0].Role == "model" {
		return append([]geminiContent{{Role: "user", Parts: []geminiPart{{Text: "(start)"}}}}, b.turns...), nil
	}
	return b.turns, nil
}

func (b *geminiTurnBuilder) systemInstruction() *geminiContent {
	if len(b.system) == 0 {
		return nil
	}
	return &geminiContent{Parts: b.system}
}

func (c *geminiContent) hasFunctionResponse() bool {
	for _, p := range c.Parts {
		if p.FunctionResponse != nil {
			return true
		}
	}
	return false
}

// lowerAssistantParts converts one assistant message into model parts: text
// first, then one functionCall part per tool call.
func lowerAssistantParts(msg openAIInboundMessage, omitCallIDs bool) []geminiPart {
	var parts []geminiPart
	if text := extractMessageText(msg.Content); text != "" {
		parts = append(parts, geminiPart{Text: text})
	}
	signatures := geminiCallSignatures(msg.ToolCalls)
	for i, call := range msg.ToolCalls {
		parts = append(parts, geminiPart{
			FunctionCall: &geminiFunctionCall{
				ID:   geminiCallID(call.ID, omitCallIDs),
				Name: call.Function.Name,
				Args: parseToolCallArgs(call.Function.Arguments),
			},
			ThoughtSignature: signatures[i],
		})
	}
	return parts
}

// geminiCallSignatures returns the thoughtSignature for each call (OpenCode
// rule): real signatures are replayed verbatim; unsigned calls before the first
// signed call get the documented skip sentinel; later unsigned calls stay empty.
func geminiCallSignatures(calls []openAIToolCall) []string {
	signatures := make([]string, len(calls))
	signed := false
	for i, call := range calls {
		switch sig := extractGoogleThoughtSignature(call.ExtraContent); {
		case sig != "":
			signatures[i] = sig
			signed = true
		case !signed:
			signatures[i] = geminiSkipThoughtSignature
		}
	}
	return signatures
}

func geminiCallID(id string, omit bool) string {
	if omit {
		return ""
	}
	return id
}

// parseToolCallArgs decodes OpenAI's JSON-string arguments; empty or invalid
// arguments become an empty object.
func parseToolCallArgs(raw string) any {
	var args any
	if json.Unmarshal([]byte(raw), &args) != nil || args == nil {
		return map[string]any{}
	}
	return args
}

// googleExtraContent is the payload of extra_content.google.
type googleExtraContent struct {
	ThoughtSignature string `json:"thought_signature"`
}

func extractGoogleThoughtSignature(extraContent map[string]json.RawMessage) string {
	var google googleExtraContent
	if json.Unmarshal(extraContent["google"], &google) != nil {
		return ""
	}
	return strings.TrimSpace(google.ThoughtSignature)
}

func googleThoughtSignatureExtra(signature string) map[string]json.RawMessage {
	raw, err := json.Marshal(googleExtraContent{ThoughtSignature: signature})
	if err != nil {
		return nil
	}
	return map[string]json.RawMessage{"google": raw}
}

func lowerGeminiTools(tools []openAIInboundTool) []geminiFunctionDeclaration {
	var decls []geminiFunctionDeclaration
	for _, t := range tools {
		if t.Function.Name == "" {
			continue
		}
		schema := t.Function.Parameters
		if len(bytes.TrimSpace(schema)) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		decls = append(decls, geminiFunctionDeclaration{
			Name:                 t.Function.Name,
			Description:          t.Function.Description,
			ParametersJSONSchema: schema,
		})
	}
	return decls
}

// lowerGeminiToolConfig maps OpenAI tool_choice: "auto"/"none"/"required" or
// {"function":{"name":...}}. A missing or unknown value sends no toolConfig.
func lowerGeminiToolConfig(raw json.RawMessage) *geminiToolConfig {
	var choice string
	if json.Unmarshal(raw, &choice) == nil {
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "auto", "":
			return &geminiToolConfig{FunctionCallingConfig: geminiFunctionCallingConfig{Mode: "AUTO"}}
		case "none":
			return &geminiToolConfig{FunctionCallingConfig: geminiFunctionCallingConfig{Mode: "NONE"}}
		case "required":
			return &geminiToolConfig{FunctionCallingConfig: geminiFunctionCallingConfig{Mode: "ANY"}}
		}
		return nil
	}
	var named struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &named) != nil || named.Function.Name == "" {
		return nil
	}
	return &geminiToolConfig{FunctionCallingConfig: geminiFunctionCallingConfig{
		Mode:                 "ANY",
		AllowedFunctionNames: []string{named.Function.Name},
	}}
}

func buildGeminiGenerationConfig(in openAIInboundRequest, candidate modelCandidate) *geminiGenerationConfig {
	cfg := &geminiGenerationConfig{
		MaxOutputTokens: clampMaxTokens(in.MaxTokens, candidate.MaxOutputTokens),
		Temperature:     in.Temperature,
		TopP:            in.TopP,
		ThinkingConfig:  geminiThinkingConfig(candidate.ExtraBody),
	}
	if cfg.MaxOutputTokens == 0 && cfg.Temperature == nil && cfg.TopP == nil && len(cfg.ThinkingConfig) == 0 {
		return nil
	}
	return cfg
}

// geminiThinkingConfig reads extra_body: "thinkingConfig" is passed through
// verbatim; otherwise a valid "reasoning_effort" maps to thinkingLevel.
func geminiThinkingConfig(extraBody map[string]json.RawMessage) json.RawMessage {
	if raw := extraBody["thinkingConfig"]; len(raw) > 0 {
		return raw
	}
	var effort string
	if json.Unmarshal(extraBody["reasoning_effort"], &effort) != nil {
		return nil
	}
	switch level := strings.ToLower(strings.TrimSpace(effort)); level {
	case "minimal", "low", "medium", "high":
		return json.RawMessage(`{"thinkingLevel":"` + level + `"}`)
	default:
		return nil
	}
}

// =============================================================================
// 3. Gemini response -> OpenAI response
// =============================================================================

// --- Gemini response ---

type geminiResponse struct {
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
	ModelVersion  string               `json:"modelVersion"`
}

type geminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	ToolUsePromptTokenCount int `json:"toolUsePromptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
}

// fromGeminiResponse converts a Gemini generateContent response into an
// OpenAI Chat Completions response. An error makes the proxy fail over.
func fromGeminiResponse(modelName string, respBody []byte) ([]byte, error) {
	var in geminiResponse
	if err := json.Unmarshal(respBody, &in); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}
	if in.Error != nil {
		return nil, fmt.Errorf("gemini error: %s", util.FirstNonEmpty(in.Error.Message, in.Error.Status, "unknown"))
	}

	parts, rawFinishReason, err := in.firstCandidate()
	if err != nil {
		return nil, err
	}
	if isGeminiErrorFinishReason(rawFinishReason) {
		return nil, fmt.Errorf("gemini stopped with %s", rawFinishReason)
	}

	text, toolCalls := convertGeminiParts(parts)
	msg := openAIChatMessage{Role: "assistant", ToolCalls: toolCalls}
	if text != "" || len(toolCalls) == 0 {
		msg.Content = &text
	}

	return json.Marshal(openAIChatResponse{
		Model: util.FirstNonEmpty(in.ModelVersion, modelName),
		Choices: []openAIChatChoice{{
			Message:      msg,
			FinishReason: mapGeminiFinishReason(rawFinishReason, len(toolCalls) > 0),
		}},
		Usage: in.UsageMetadata.toOpenAI(),
	})
}

// firstCandidate returns the parts and finish reason of the first candidate.
// A blocked prompt has no candidates and is reported as SAFETY (OpenCode
// treats it as a content-filter stop, not an upstream failure).
func (r geminiResponse) firstCandidate() ([]geminiPart, string, error) {
	if len(r.Candidates) > 0 {
		return r.Candidates[0].Content.Parts, r.Candidates[0].FinishReason, nil
	}
	if r.PromptFeedback != nil && r.PromptFeedback.BlockReason != "" {
		return nil, "SAFETY", nil
	}
	return nil, "", errors.New("gemini returned no candidates")
}

// convertGeminiParts joins visible text and converts function calls; thought
// parts are internal reasoning and are dropped.
func convertGeminiParts(parts []geminiPart) (string, []openAIToolCall) {
	var text strings.Builder
	var toolCalls []openAIToolCall
	seenIDs := make(map[string]bool)
	for _, part := range parts {
		switch {
		case part.Thought:
		case part.FunctionCall != nil:
			toolCalls = append(toolCalls, toOpenAIToolCall(part, seenIDs))
		default:
			text.WriteString(part.Text)
		}
	}
	return text.String(), toolCalls
}

// toOpenAIToolCall keeps Gemini's call ID unless it is missing or repeated,
// and carries the thought signature back in extra_content for the next turn.
func toOpenAIToolCall(part geminiPart, seenIDs map[string]bool) openAIToolCall {
	id := strings.TrimSpace(part.FunctionCall.ID)
	if id == "" || seenIDs[id] {
		id = "call_" + security.RandomStringWithAlphabet(16, recordIDAlphabet)
	}
	seenIDs[id] = true

	args := part.FunctionCall.Args
	if args == nil {
		args = map[string]any{}
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		argsJSON = []byte("{}")
	}

	call := openAIToolCall{
		ID:       id,
		Type:     "function",
		Function: openAIFunctionCall{Name: part.FunctionCall.Name, Arguments: string(argsJSON)},
	}
	if sig := strings.TrimSpace(part.ThoughtSignature); sig != "" {
		call.ExtraContent = googleThoughtSignatureExtra(sig)
	}
	return call
}

// mapGeminiFinishReason mirrors OpenCode's mapFinishReason priority.
func mapGeminiFinishReason(raw string, hasToolCalls bool) string {
	switch {
	case raw == "MAX_TOKENS":
		return "length"
	case isGeminiContentFilterFinishReason(raw):
		return "content_filter"
	case hasToolCalls:
		return "tool_calls"
	default:
		return "stop"
	}
}

// isGeminiContentFilterFinishReason matches OpenCode's CONTENT_FILTER_REASONS.
func isGeminiContentFilterFinishReason(reason string) bool {
	switch reason {
	case "IMAGE_SAFETY", "RECITATION", "SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII",
		"MODEL_ARMOR", "IMAGE_PROHIBITED_CONTENT", "IMAGE_RECITATION", "LANGUAGE":
		return true
	default:
		return false
	}
}

// isGeminiErrorFinishReason lists finish reasons OpenCode treats as errors.
func isGeminiErrorFinishReason(reason string) bool {
	switch reason {
	case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL", "TOO_MANY_TOOL_CALLS",
		"MISSING_THOUGHT_SIGNATURE", "MALFORMED_RESPONSE":
		return true
	default:
		return false
	}
}

// toOpenAI reports tool-use prompt tokens as input and thinking tokens as
// output, so usage accounting matches Gemini billing.
func (u *geminiUsageMetadata) toOpenAI() *openAIUsage {
	if u == nil {
		return nil
	}
	prompt := max(u.PromptTokenCount+u.ToolUsePromptTokenCount, 0)
	completion := max(u.CandidatesTokenCount+u.ThoughtsTokenCount, 0)
	total := max(u.TotalTokenCount, 0)
	if total == 0 {
		total = prompt + completion
	}
	return &openAIUsage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
}
