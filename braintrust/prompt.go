package braintrust

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Prompt struct {
	Name           string
	Description    string
	Version        string
	Model          string
	Messages       []Message
	Temperature    *float32
	MaxTokens      *int64
	TopP           *float32
	TopK           *int32
	UseCache       *bool
	ToolChoice     *string // "none", "auto", "required", "function"
	ToolFunction   *string
	ResponseFormat *Response
	Reasoning      *Reasoning
	Metadata       *Metadata
}

type Message struct {
	Role    Role
	Content string
}

type Metadata struct {
	AutoApproveTools []string `json:"auto_approve_tools,omitempty"`
	Betas            []string `json:"betas,omitempty"`
	Skills           []Skill  `json:"skills,omitempty"`
	Tools            []Tool   `json:"tools,omitempty"`
}

type Skill struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type,omitempty"`
	Version string `json:"version,omitempty"`
}

type Tool struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

type Response struct {
	Type   string  `json:"type,omitempty"` // text, json_object, json_schema
	Schema *Schema `json:"schema,omitempty"`
}

type Schema struct {
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type Reasoning struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Budget  *int64  `json:"budget,omitempty"`
	Effort  *string `json:"effort,omitempty"` // "low", "medium", "high" (OpenAI specific)
}
