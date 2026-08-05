package agui

import (
	"strconv"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

func (e *encoder) Finish(terminal error) ([]uistream.Frame, error) {
	frames, err := e.closeOpenContent()
	if err != nil {
		return nil, err
	}
	e.commitStepUsage()

	if terminal != nil {
		// threadId and runId are stamped so the client scopes the failure to
		// this run; a run-less RUN_ERROR clears every active run.
		last, err := e.event(eventRunError, map[string]any{
			"message":  uistream.RedactStreamError(terminal),
			"threadId": e.threadID,
			"runId":    e.runID,
		})
		return append(frames, last...), err
	}

	if len(e.interrupts) > 0 {
		return e.finishInterrupted(frames)
	}
	return e.finishSuccess(frames)
}

func (e *encoder) finishSuccess(frames []uistream.Frame) ([]uistream.Frame, error) {
	data := map[string]any{"threadId": e.threadID, "runId": e.runID}
	if e.finishReason != "" {
		data["finishReason"] = e.finishReason
	}
	if e.totalUsage.HasValues() {
		data["usage"] = usagePayload(e.totalUsage)
	}
	last, err := e.event(eventRunFinished, data)
	return append(frames, last...), err
}

func (e *encoder) textDelta(delta string) ([]uistream.Frame, error) {
	// Reasoning and assistant text are distinct AG-UI messages; close the
	// thinking message before the answer begins.
	frames, err := e.closeReasoning()
	if err != nil {
		return nil, err
	}
	e.textBuf.WriteString(delta)
	if !e.textOpen {
		e.textOpen = true
		e.textID = e.messageID()
		start, err := e.event(eventTextMessageStart,
			map[string]any{"messageId": e.textID, "role": "assistant"})
		if err != nil {
			return nil, err
		}
		frames = append(frames, start...)
	}
	content, err := e.event(eventTextMessageContent,
		map[string]any{"messageId": e.textID, "delta": delta})
	return append(frames, content...), err
}

// closeOpenContent closes every open block in the order the client expects:
// reasoning, then text, then any tool still streaming arguments.
func (e *encoder) closeOpenContent() ([]uistream.Frame, error) {
	frames, err := e.closeReasoning()
	if err != nil {
		return nil, err
	}
	if e.textOpen {
		end, err := e.event(eventTextMessageEnd, map[string]any{"messageId": e.textID})
		if err != nil {
			return nil, err
		}
		frames = append(frames, end...)
		e.textOpen = false
	}
	for _, id := range e.toolOrder {
		end, err := e.closeTool(id)
		if err != nil {
			return nil, err
		}
		frames = append(frames, end...)
	}
	return frames, nil
}

func (e *encoder) commitStepUsage() {
	if e.stepUsage.HasValues() || e.stepUsage.Raw != nil {
		e.totalUsage = e.totalUsage.Add(e.stepUsage)
		e.stepUsage = aikit.Usage{}
	}
}

func (e *encoder) stepName() string { return "step_" + strconv.Itoa(e.step) }

func (e *encoder) messageID() string {
	return e.runID + "_message_" + strconv.Itoa(e.step)
}
