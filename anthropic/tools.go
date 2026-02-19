package anthropic

import "github.com/eolymp/go-agent"

// WithBashTool adds Anthropic's bash code execution tool to the agent.
// This tool is automatically handled by Anthropic's backend and is available in both beta and non-beta APIs.
// It's required when using skills in a container.
func WithBashTool() agent.Option {
	return agent.WithBuiltinTool("bash", "bash_20250124")
}

// WithCodeExecutionTool adds Anthropic's code execution tool to the agent (beta only).
// This tool is automatically handled by Anthropic's backend and requires the code-execution-2025-08-25 beta flag.
// The beta flag is automatically added when using this tool.
func WithCodeExecutionTool() agent.Option {
	return agent.WithOptions(
		agent.WithBuiltinTool("code_execution", "code_execution_20250825"),
		agent.WithBetas("code-execution-2025-08-25"),
	)
}

// WithAdvancedCodeExecutionTool adds Anthropic's code execution tool with advanced-tool-use support.
// This tool requires the advanced-tool-use-2025-11-20 beta flag.
// The beta flag is automatically added when using this tool.
func WithAdvancedCodeExecutionTool() agent.Option {
	return agent.WithOptions(
		agent.WithBuiltinTool("code_execution", "code_execution_20250825"),
		agent.WithBetas("advanced-tool-use-2025-11-20"),
	)
}

// WithWebSearchTool adds Anthropic's web search tool to the agent.
// This tool is automatically handled by Anthropic's backend.
func WithWebSearchTool() agent.Option {
	return agent.WithBuiltinTool("web_search", "web_search_20250305")
}

// WithToolSearchRegex adds Anthropic's regex-based tool search tool to the agent (beta only).
// This tool is automatically handled by Anthropic's backend.
func WithToolSearchRegex() agent.Option {
	return agent.WithBuiltinTool("tool_search_tool_regex", "tool_search_tool_regex_20251119")
}

// WithToolSearchBM25 adds Anthropic's BM25-based tool search tool to the agent (beta only).
// This tool is automatically handled by Anthropic's backend.
func WithToolSearchBM25() agent.Option {
	return agent.WithBuiltinTool("tool_search_tool_bm25", "tool_search_tool_bm25_20251119")
}

// WithSkills adds skills to the agent container with the required skills beta flag.
// The skills-2025-10-02 beta flag is automatically added when using this option.
func WithSkills(skills ...agent.Skill) agent.Option {
	return agent.WithOptions(
		agent.WithContainer(&agent.Container{Skills: skills}),
		agent.WithBetas("skills-2025-10-02"),
	)
}
