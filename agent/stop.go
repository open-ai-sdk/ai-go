package agent

// IsStepCount stops after n completed tool-loop steps.
func IsStepCount(n int) StopCondition {
	return func(step int, _ *StepResult) bool {
		return step >= n
	}
}

// Never never stops a run early. The model ending without a tool call still
// terminates naturally, and a positive RunParams.MaxSteps remains a hard cap.
func Never() StopCondition {
	return func(_ int, _ *StepResult) bool {
		return false
	}
}

// HasToolCall stops after a step that called toolName.
func HasToolCall(toolName string) StopCondition {
	return func(_ int, result *StepResult) bool {
		if result == nil {
			return false
		}
		for _, name := range result.ToolNames {
			if name == toolName {
				return true
			}
		}
		return false
	}
}
