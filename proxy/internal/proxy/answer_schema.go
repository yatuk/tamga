package proxy

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// output_rules.json_schema holds an answer that is meant to be JSON to a
// schema. What is judged is the message the model wrote, once per choice:
//
//   - OpenAI chat: choices[].message.content;
//   - OpenAI Responses: the output_text parts of each message in output[];
//   - Anthropic: the text blocks of content[], joined;
//   - Gemini: the text parts of candidates[].content.parts[], joined.
//
// A tool call's arguments are not judged: the schema they answer to is the
// tool's, which the caller declared in the request and the policy does not
// know. A message with no text (a tool call and nothing else), a refusal the
// provider marks as one, and reasoning the provider keeps apart from the
// answer are not judged either.
//
// The text has to be the document and nothing else. JSON inside a code
// fence or after a sentence of introduction is not_json, as it is to the
// program that will parse it.

type answerSchemaKey struct{}

// answerSchemaChoice is the decision made for one request, carried to where
// its answer is scanned. A nil schema means the answer is not checked.
type answerSchemaChoice struct {
	schema *scanner.Schema
}

// withAnswerSchema decides, from the request, whether its answer is held to
// the policy's schema, and puts the decision on the context.
func withAnswerSchema(ctx context.Context, pol *policy.Policy, requestBody []byte) context.Context {
	return context.WithValue(ctx, answerSchemaKey{}, answerSchemaChoice{schema: schemaForRequest(pol, requestBody)})
}

// schemaForRequest returns the schema the answer to this request is held
// to, or nil.
func schemaForRequest(pol *policy.Policy, requestBody []byte) *scanner.Schema {
	schema, always := pol.AnswerSchema()
	if schema == nil {
		return nil
	}
	if always || jsonRequested(requestBody) {
		return schema
	}
	return nil
}

// answerSchema returns the schema an answer scanned under ctx is held to.
// Where nothing was decided from a request (the LiteLLM bridge is handed an
// answer without the request it answers), the schema applies only when the
// policy applies it to every answer.
func answerSchema(ctx context.Context, pol *policy.Policy) *scanner.Schema {
	if choice, ok := ctx.Value(answerSchemaKey{}).(answerSchemaChoice); ok {
		return choice.schema
	}
	if schema, always := pol.AnswerSchema(); always {
		return schema
	}
	return nil
}

// jsonRequested reports whether a request asks the model for a JSON
// document, in any of the ways the providers have for it.
func jsonRequested(body []byte) bool {
	var req struct {
		// OpenAI chat completions.
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		// OpenAI Responses.
		Text struct {
			Format struct {
				Type string `json:"type"`
			} `json:"format"`
		} `json:"text"`
		// Gemini, in both spellings its API accepts.
		GenerationConfig struct {
			ResponseMimeType  string `json:"responseMimeType"`
			ResponseMimeType2 string `json:"response_mime_type"`
		} `json:"generationConfig"`
		GenerationConfig2 struct {
			ResponseMimeType  string `json:"responseMimeType"`
			ResponseMimeType2 string `json:"response_mime_type"`
		} `json:"generation_config"`
		// Anthropic structured outputs.
		OutputConfig struct {
			Format struct {
				Type string `json:"type"`
			} `json:"format"`
		} `json:"output_config"`
		OutputFormat struct {
			Type string `json:"type"`
		} `json:"output_format"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	jsonType := func(t string) bool { return t == "json_object" || t == "json_schema" }
	jsonMime := func(m string) bool { return strings.EqualFold(strings.TrimSpace(m), "application/json") }
	return jsonType(req.ResponseFormat.Type) || jsonType(req.Text.Format.Type) ||
		jsonType(req.OutputConfig.Format.Type) || jsonType(req.OutputFormat.Type) ||
		jsonMime(req.GenerationConfig.ResponseMimeType) || jsonMime(req.GenerationConfig.ResponseMimeType2) ||
		jsonMime(req.GenerationConfig2.ResponseMimeType) || jsonMime(req.GenerationConfig2.ResponseMimeType2)
}

// answerDocument is the text of one message of an answer and where it is.
type answerDocument struct {
	path string
	text string
}

// answerDocuments returns the messages of a buffered answer that the schema
// is to judge. It returns nothing for a shape it does not know.
func answerDocuments(raw []byte) []answerDocument {
	var body struct {
		Choices []struct {
			Message *struct {
				Content json.RawMessage `json:"content"`
				Refusal json.RawMessage `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    *string `json:"text"`
					Thought bool    `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil
	}
	var docs []answerDocument
	add := func(path, text string) {
		if strings.TrimSpace(text) != "" {
			docs = append(docs, answerDocument{path: path, text: text})
		}
	}
	textParts := func(raw json.RawMessage, types ...string) (string, bool) {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &parts) != nil {
			return "", false
		}
		var b strings.Builder
		for _, p := range parts {
			for _, t := range types {
				if p.Type == t {
					b.WriteString(p.Text)
				}
			}
		}
		return b.String(), true
	}

	for i, c := range body.Choices {
		if c.Message == nil {
			continue
		}
		if refusal := strings.TrimSpace(string(c.Message.Refusal)); refusal != "" && refusal != "null" && refusal != `""` {
			continue
		}
		path := "choices[" + strconv.Itoa(i) + "].message.content"
		var text string
		if json.Unmarshal(c.Message.Content, &text) == nil {
			add(path, text)
		} else if joined, ok := textParts(c.Message.Content, "text", "output_text"); ok {
			add(path, joined)
		}
	}
	for i, item := range body.Output {
		if item.Type != "message" {
			continue
		}
		var b strings.Builder
		refused := false
		for _, part := range item.Content {
			switch part.Type {
			case "output_text":
				b.WriteString(part.Text)
			case "refusal":
				refused = true
			}
		}
		if !refused {
			add("output["+strconv.Itoa(i)+"].content", b.String())
		}
	}
	if len(body.Choices) == 0 && len(body.Output) == 0 && len(body.Content) > 0 && body.StopReason != "refusal" {
		if joined, ok := textParts(body.Content, "text"); ok {
			add("content", joined)
		}
	}
	for i, c := range body.Candidates {
		var b strings.Builder
		for _, part := range c.Content.Parts {
			if part.Text != nil && !part.Thought {
				b.WriteString(*part.Text)
			}
		}
		add("candidates["+strconv.Itoa(i)+"].content.parts", b.String())
	}
	return docs
}

// schemaFindings judges the messages of a buffered answer. The error is the
// context's: the check was cut short.
func schemaFindings(ctx context.Context, schema *scanner.Schema, raw []byte) ([]scanner.Finding, error) {
	var findings []scanner.Finding
	for _, doc := range answerDocuments(raw) {
		f, bad, err := judgeDocument(ctx, schema, doc.text)
		if err != nil {
			return findings, err
		}
		if bad {
			f.Path = doc.path
			findings = append(findings, f)
		}
	}
	return findings, nil
}

// judgeDocument checks one message. bad is true when it was not accepted.
func judgeDocument(ctx context.Context, schema *scanner.Schema, text string) (f scanner.Finding, bad bool, err error) {
	res, err := schema.Check(ctx, []byte(text))
	if err != nil || res.OK() {
		return scanner.Finding{}, false, err
	}
	f = scanner.SchemaFinding(res)
	f.Role = string(extract.RoleAssistant)
	return f, true, nil
}

// defaultSchemaStreamBytes is how much of a streamed answer is kept to be
// judged when output_rules.buffer_bytes sets no limit: the limit a buffered
// answer has by default.
const defaultSchemaStreamBytes = maxOutputScanBytes

// configureStreamSchema sets a stream guard up to judge the answer it
// carries, when the answer to this request is held to a schema.
//
// A document can only be judged whole. When a mismatch is to withhold the
// answer (block_on names it), the text is therefore kept back until the
// model has finished and is sent in one piece, or not at all: the client
// receives the stream's events as they come, with no text in them, and the
// whole text at the end. When a mismatch is only recorded, the text streams
// as it always does and is judged when it ends; the client has all of it by
// then.
func configureStreamSchema(ctx context.Context, g *streamGuard, pol *policy.Policy, requestBody []byte) {
	schema := schemaForRequest(pol, requestBody)
	if schema == nil {
		return
	}
	g.schema = func(text string) (scanner.Finding, bool, error) {
		return judgeDocument(ctx, schema, text)
	}
	g.schemaHold = pol.OutputSchemaBlocks()
	g.schemaMax = defaultSchemaStreamBytes
	if pol.OutputRules != nil && pol.OutputRules.BufferBytes > 0 {
		g.schemaMax = pol.OutputRules.BufferBytes
	}
}
