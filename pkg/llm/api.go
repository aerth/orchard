// Package llm (ollama api) wrapper containing [Client] [Message] [ChatRequest] etc
package llm

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"m/pkg/global"

	"github.com/aerth/mostly/stackerr"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Client struct {
	base *url.URL
	http *http.Client
}

func NewClient(uRL *url.URL, client *http.Client) *Client {
	return &Client{base: uRL, http: client}
}

func New() *Client {
	c, err := ClientFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func ClientFromEnvironment() (*Client, error) {
	host := getHost()
	if strings.Contains(host.Host, "ollama.com") {
		return nil, fmt.Errorf("ollama.com detected")
	}
	return &Client{
		base: host,
		http: http.DefaultClient,
	}, nil
}

func (c *Client) do(ctx context.Context, method, path string, reqData, respData any) error {
	var reqBody io.Reader
	var data []byte
	var err error

	switch reqData := reqData.(type) {
	case io.Reader:
		// reqData is already an io.Reader
		reqBody = reqData
	case nil:
		// noop
	default:
		data, err = json.Marshal(reqData)
		if err != nil {
			return err
		}

		reqBody = bytes.NewReader(data)
	}

	requestURL := c.base.JoinPath(path)

	var token string
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reqBody)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", fmt.Sprintf("orchard/%s (%s %s) Go/%s", "0.0.0", runtime.GOARCH, runtime.GOOS, global.GetVersion()))

	if token != "" {
		request.Header.Set("Authorization", token)
	}

	respObj, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		_ = respObj.Body.Close()
	}()

	respBody, err := io.ReadAll(respObj.Body)
	if err != nil {
		return err
	}

	if err := checkError(respObj, respBody); err != nil {
		return err
	}

	if len(respBody) > 0 && respData != nil {
		if err := json.Unmarshal(respBody, respData); err != nil {
			return err
		}
	}
	return nil
}

func checkError(resp *http.Response, body []byte) error {
	if resp.StatusCode < http.StatusBadRequest {
		return nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		authError := AuthorizationError{StatusCode: resp.StatusCode}
		err := json.Unmarshal(body, &authError)
		if err != nil {
			return stackerr.Errorf("unmarshal json: %w", err)
		}

		return authError
	}

	apiError := StatusError{StatusCode: resp.StatusCode}

	err := json.Unmarshal(body, &apiError)
	if err != nil {
		// Use the full body as the message if we fail to decode a response.
		apiError.ErrorMessage = string(body)
	}

	return apiError
}

// Embed generates embeddings from a model.
func (c *Client) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	var resp EmbedResponse
	if err := c.do(ctx, http.MethodPost, "/api/embed", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// EmbedRequest is the request passed to [Client.Embed].
type EmbedRequest struct {
	Model     string `json:"model"`
	Input     any    `json:"input"`
	KeepAlive int    `json:"keep_alive,omitempty"`
	// Dimensions truncates the output embedding to the specified dimension.
	Dimensions int   `json:"dimensions,omitempty"`
	Truncate   *bool `json:"truncate,omitempty"`
	// Options lists model-specific options.
	Options map[string]any `json:"options"`
}

// EmbedResponse is the response from [Client.Embed].
type EmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`

	TotalDuration   time.Duration `json:"total_duration,omitempty"`
	LoadDuration    time.Duration `json:"load_duration,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
}

type Message struct {
	Role       string    `json:"role"`
	Thinking   string    `json:"thinking,omitempty" gorm:"null"`
	Content    string    `json:"content,omitempty"`
	ToolCalls  ToolCalls `json:"tool_calls,omitempty" gorm:"type:jsonb;null"`
	ToolName   string    `json:"tool_name,omitempty" gorm:"null"`
	ToolCallID string    `json:"tool_call_id,omitempty" gorm:"null"`
}

var caser = cases.Title(language.AmericanEnglish)

func title(in string) string {
	if in == "" {
		return in
	}
	return caser.String(in)
}

func (m Message) String() string {
	if len(m.ToolCalls) != 0 {
		return fmt.Sprintf("%s tool calls: %s", title(m.Role), m.ToolCalls)
	}
	if m.Content == "" {
		return fmt.Sprintf("%s: (thinking) %s", title(m.Role), m.Thinking)
	}
	return fmt.Sprintf("%s: %s", m.Role, m.Content)
}

type (
	Messages  []Message
	ToolCalls []json.RawMessage
)

// Value implements the driver.Valuer interface (for Writing to DB)
func (tc ToolCalls) Value() (driver.Value, error) {
	if tc == nil {
		return nil, nil
	}
	return json.Marshal(tc)
}

// Scan implements the sql.Scanner interface (for Reading from DB)
func (tc *ToolCalls) Scan(value interface{}) error {
	if value == nil {
		*tc = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		// In some drivers/versions, it might come as []uint8
		uBytes, ok := value.([]uint8)
		if !ok {
			return errors.New("type assertion to []byte failed")
		}
		bytes = uBytes
	}

	return json.Unmarshal(bytes, tc)
}

func toolCallsEqual(a, b ToolCalls) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal([]byte(a[i]), []byte(b[i])) {
			return false
		}
	}
	return true
}

func (m Message) Equals(msg Message) bool {
	return m.Content == msg.Content && m.Thinking == msg.Thinking && toolCallsEqual(m.ToolCalls, msg.ToolCalls)
}

type Message9 struct {
	Role      string
	Thinking  string
	Content   string
	ToolCalls []json.RawMessage
}
type ChatResponse struct {
	// Model is the model name that generated the response.
	Model string `json:"model"`

	// RemoteModel is the name of the upstream model that generated the response.
	RemoteModel string `json:"remote_model,omitempty"`

	// RemoteHost is the URL of the upstream Ollama host that generated the response.
	RemoteHost string `json:"remote_host,omitempty"`

	// CreatedAt is the timestamp of the response.
	CreatedAt time.Time `json:"created_at"`

	// Message contains the message or part of a message from the model.
	Message Message `json:"message"`

	// Done specifies if the response is complete.
	Done bool `json:"done"`

	// DoneReason is the reason the model stopped generating text.
	DoneReason any `json:"done_reason,omitempty"`

	// DebugInfo *DebugInfo `json:"_debug_info,omitempty"`

	// Logprobs contains log probability information for the generated tokens,
	// if requested via the Logprobs parameter.
	// Logprobs []Logprob `json:"logprobs,omitempty"`

	// Metrics
}

type ChatRequest struct {
	Model     string           `json:"model"`
	Think     any              `json:"think,omitempty"`
	Stream    *bool            `json:"stream,omitempty"`
	Messages  []Message        `json:"messages"`
	Tools     []json.Marshaler `json:"tools,omitempty"`
	KeepAlive int              `json:"keep_alive"`
	Options   map[string]any   `json:"options"`
}

func isFirstCharJSON(c byte) bool {
	return c == '{' || c == '['
}

type ChatStreamHandlerFunc = func(ChatResponse) error

func (c Client) Chat(ctx context.Context, creq *ChatRequest, callbacks ...ChatStreamHandlerFunc) error {
	if len(callbacks) == 0 {
		return fmt.Errorf("no callbacks")
	}
	return c.stream(ctx, http.MethodPost, "/api/chat", creq, func(bts []byte) error {
		if len(bts) == 0 || !isFirstCharJSON(bts[0]) {
			return stackerr.Errorf("server replied not with json object: %s", string(bts))
		}
		var resp ChatResponse
		err := json.Unmarshal(bts, &resp)
		if err != nil {
			return stackerr.Errorf("json decoding %s %w", string(bts), err)
		}
		for i := range callbacks {
			if err := callbacks[i](resp); err != nil {
				return err
			}
		}
		return nil
	})
}

var (
	DefaultModel = "gemma4:26b" // set by flag
	DefaultThink = "low"
)

// NewChatRequest from env
func NewChatRequest() *ChatRequest {
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = DefaultModel
	}
	temp := 0.1
	if strings.HasPrefix(model, "gemma") {
		temp = 1.0
	}
	var think any = os.Getenv("OLLAMA_THINK")
	if think == "" {
		think = DefaultThink
	}
	if think == "false" {
		think = false
	}
	if think == "true" {
		think = true
	}
	return &ChatRequest{
		Model:     model,
		Think:     think,
		Stream:    new(true),
		KeepAlive: -1,
		//	DebugRenderOnly: os.Getenv("OLLAMA_DEBUG_RENDER") == "1",
		//	TopLogprobs:     0,
		Options: map[string]any{"temperature": temp}, // "num_ctx": numCtx},
	}
}
