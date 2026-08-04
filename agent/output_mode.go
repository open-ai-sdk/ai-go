package agent

import (
	"fmt"

	"github.com/open-ai-sdk/ai-go/llm"
)

// OutputMode chooses how an agent enforces a configured output schema.
type OutputMode string

const (
	OutputModeAuto   OutputMode = "auto"
	OutputModeNative OutputMode = "native"
	OutputModeTool   OutputMode = "tool"
)

func resolveOutputMode(requested OutputMode, hasSchema, hasTools, outputToolCallable bool, native llm.NativeSchemaSupport) (OutputMode, error) {
	if !hasSchema {
		return OutputModeNative, nil
	}
	if requested == "" {
		requested = OutputModeAuto
	}
	switch requested {
	case OutputModeNative:
		if native == llm.NativeSchemaNone || (native == llm.NativeSchemaSuppressesTools && hasTools) {
			return "", fmt.Errorf("native structured output cannot satisfy this tool configuration")
		}
		return OutputModeNative, nil
	case OutputModeTool:
		if !outputToolCallable {
			return "", fmt.Errorf("output tool is forbidden by tool choice")
		}
		return OutputModeTool, nil
	case OutputModeAuto:
		if native == llm.NativeSchemaFull {
			return OutputModeNative, nil
		}
		if outputToolCallable {
			return OutputModeTool, nil
		}
		if native == llm.NativeSchemaSuppressesTools && !hasTools {
			return OutputModeNative, nil
		}
		return "", fmt.Errorf("no structured-output mode can satisfy this configuration")
	default:
		return "", fmt.Errorf("unknown output mode %q", requested)
	}
}

func outputToolCallable(choice *ToolChoice, name string) bool {
	return choice == nil || choice.Type != "none" && (choice.Type != "tool" || choice.ToolName == name)
}
