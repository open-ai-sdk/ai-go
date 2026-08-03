package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

func mustHookToolSet(t *testing.T, tools ...tool.Invokable) *tool.Set {
	t.Helper()
	set, err := tool.NewSet(tools...)
	if err != nil {
		t.Fatalf("NewSet() error = %v", err)
	}
	return set
}

func hookDynamicTool(
	t *testing.T,
	name string,
	execute func(context.Context, json.RawMessage) (json.RawMessage, error),
) tool.Invokable {
	t.Helper()
	value, err := tool.NewDynamic(name, "hook test tool", map[string]any{"type": "object"}, execute)
	if err != nil {
		t.Fatalf("NewDynamic(%q) error = %v", name, err)
	}
	return value
}

func TestRunnerHooksUseStableContextAndPerTurnCompletionPatches(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{
		{
			{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "call-1", ToolCallName: "lookup", ToolCallArgsDelta: `{}`},
			{Type: aikit.StreamEventFinish, MessageID: "tool-turn", FinishReason: aikit.FinishReasonToolCalls},
		},
		runnerTextEvents("answer", "done"),
	}}
	set := mustHookToolSet(t, hookDynamicTool(t, "lookup", func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	}))

	var mu sync.Mutex
	var completionContexts []agent.HookContext
	var completionInputs []string
	var eventOrder []string
	var finishedContext agent.HookContext
	first := agent.HookFuncs{
		Name: "first",
		BeforeCompletionFunc: func(_ context.Context, hc agent.HookContext, request llm.Request) (agent.CompletionAction, error) {
			mu.Lock()
			completionContexts = append(completionContexts, hc)
			completionInputs = append(completionInputs, request.Instructions)
			mu.Unlock()
			patched := fmt.Sprintf("hook-turn-%d", hc.Turn)
			return agent.CompletionAction{
				Kind:  agent.CompletionPatch,
				Patch: agent.RequestPatch{Instructions: &patched},
			}, nil
		},
		StreamEventFunc: func(_ context.Context, _ agent.HookContext, event aikit.StepEvent) error {
			mu.Lock()
			eventOrder = append(eventOrder, fmt.Sprintf("first:%d", event.Type))
			mu.Unlock()
			return nil
		},
		RunFinishedFunc: func(_ context.Context, hc agent.HookContext, _ *agent.Result, _ error) {
			mu.Lock()
			finishedContext = hc
			mu.Unlock()
		},
	}
	second := agent.HookFuncs{
		Name: "second",
		BeforeCompletionFunc: func(_ context.Context, _ agent.HookContext, request llm.Request) (agent.CompletionAction, error) {
			if request.Instructions == "" {
				t.Error("second hook did not receive first hook's patch")
			}
			return agent.CompletionAction{Kind: agent.CompletionContinue}, nil
		},
		StreamEventFunc: func(_ context.Context, _ agent.HookContext, event aikit.StepEvent) error {
			mu.Lock()
			eventOrder = append(eventOrder, fmt.Sprintf("second:%d", event.Type))
			mu.Unlock()
			return nil
		},
	}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
		return builder.ID("hook-agent").Tools(set).MaxTurns(2).
			PrepareStep(func(input llm.PrepareStepContext) *llm.PrepareStepResult {
				return &llm.PrepareStepResult{Instructions: fmt.Sprintf("prepared-%d", input.StepNumber+1)}
			}).
			Hook(first).Hook(second)
	})

	result, err := built.Runner().Prompt("go").Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "done" {
		t.Fatalf("Run() text = %q, want done", result.Text)
	}
	requests := model.requestSnapshots()
	if len(requests) != 2 || requests[0].Instructions != "hook-turn-1" || requests[1].Instructions != "hook-turn-2" {
		t.Fatalf("model instructions = %#v", requests)
	}
	if !reflect.DeepEqual(completionInputs, []string{"prepared-1", "prepared-2"}) {
		t.Fatalf("completion hook inputs = %v", completionInputs)
	}
	if len(completionContexts) != 2 {
		t.Fatalf("completion contexts = %#v", completionContexts)
	}
	for i, hc := range completionContexts {
		if hc.RunID == "" || hc.Turn != i+1 || hc.Streaming || hc.AgentID != "hook-agent" {
			t.Fatalf("completion context %d = %#v", i, hc)
		}
	}
	if completionContexts[0].RunID != completionContexts[1].RunID || finishedContext.RunID != completionContexts[0].RunID {
		t.Fatalf("run IDs are not stable: completion=%#v finished=%#v", completionContexts, finishedContext)
	}
	if finishedContext.Turn != 2 || finishedContext.Streaming || finishedContext.AgentID != "hook-agent" {
		t.Fatalf("finished context = %#v", finishedContext)
	}
	if len(eventOrder) == 0 || len(eventOrder)%2 != 0 {
		t.Fatalf("event hook order = %v", eventOrder)
	}
	for i := 0; i < len(eventOrder); i += 2 {
		if eventOrder[i][:6] != "first:" || eventOrder[i+1][:7] != "second:" || eventOrder[i][6:] != eventOrder[i+1][7:] {
			t.Fatalf("event hook registration order at %d = %v", i, eventOrder[i:i+2])
		}
	}
}

func TestRunnerHookErrorIsTypedAndObserverPanicIsRecovered(t *testing.T) {
	sentinel := errors.New("policy unavailable")
	blockedModel := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("unused", "unused")}}
	blocked := mustRunnerAgent(t, blockedModel, func(builder agent.Builder) agent.Builder {
		return builder.Hook(agent.HookFuncs{
			Name: "policy",
			BeforeCompletionFunc: func(context.Context, agent.HookContext, llm.Request) (agent.CompletionAction, error) {
				return agent.CompletionAction{}, sentinel
			},
		})
	})
	_, err := blocked.Runner().Prompt("blocked").Run(context.Background())
	var hookErr *agent.HookError
	if !errors.As(err, &hookErr) || !errors.Is(err, sentinel) || hookErr.Hook != "policy" || hookErr.Phase != "before_completion" {
		t.Fatalf("Run() error = %T %v, want typed policy HookError", err, err)
	}
	if len(blockedModel.requestSnapshots()) != 0 {
		t.Fatal("model was called after steering hook failure")
	}

	panicModel := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("answer", "ok")}}
	panickingObserver := mustRunnerAgent(t, panicModel, func(builder agent.Builder) agent.Builder {
		return builder.Hook(agent.HookFuncs{
			Name: "observer",
			StreamEventFunc: func(context.Context, agent.HookContext, aikit.StepEvent) error {
				panic("observer panic")
			},
			RunFinishedFunc: func(context.Context, agent.HookContext, *agent.Result, error) {
				panic("finished observer panic")
			},
		})
	})
	result, err := panickingObserver.Runner().Prompt("continue").Run(context.Background())
	if err != nil || result.Text != "ok" {
		t.Fatalf("observer panic affected Run(): result=%#v err=%v", result, err)
	}
}

func TestRunnerToolHooksRewriteSkipAndRepair(t *testing.T) {
	t.Run("sequential rewrite", func(t *testing.T) {
		var executed json.RawMessage
		set := mustHookToolSet(t, hookDynamicTool(t, "echo", func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
			executed = append(json.RawMessage(nil), input...)
			return input, nil
		}))
		model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{{
			{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "echo-1", ToolCallName: "echo", ToolCallArgsDelta: `{"value":"original"}`},
			{Type: aikit.StreamEventFinish, MessageID: "tool-turn", FinishReason: aikit.FinishReasonToolCalls},
		}}}
		built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
			return builder.Tools(set).Hook(agent.HookFuncs{
				Name: "rewrite",
				BeforeToolFunc: func(context.Context, agent.HookContext, aikit.ToolCallInfo) (agent.ToolCallAction, error) {
					return agent.ToolCallAction{Kind: agent.ToolCallRewrite, Args: json.RawMessage(`{"value":"rewritten"}`)}, nil
				},
				AfterToolFunc: func(context.Context, agent.HookContext, aikit.ToolResult) (agent.ToolResultAction, error) {
					return agent.ToolResultAction{Kind: agent.ToolResultRewrite, Result: aikit.ToolResult{Output: "after"}}, nil
				},
			})
		})
		result, err := built.Runner().Prompt("call").Run(context.Background())
		var maxTurns *agent.MaxTurnsError
		if !errors.As(err, &maxTurns) || result == nil {
			t.Fatalf("Run() = (%#v, %v), want partial MaxTurnsError", result, err)
		}
		if string(executed) != `{"value":"rewritten"}` || len(result.ToolResults) != 1 || result.ToolResults[0].Output != "after" {
			t.Fatalf("rewrite result: executed=%s results=%#v", executed, result.ToolResults)
		}
	})

	t.Run("parallel skip", func(t *testing.T) {
		var skipExecutions atomic.Int32
		set := mustHookToolSet(t,
			hookDynamicTool(t, "skip", func(context.Context, json.RawMessage) (json.RawMessage, error) {
				skipExecutions.Add(1)
				return json.RawMessage(`"unexpected"`), nil
			}),
			hookDynamicTool(t, "run", func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`"ran"`), nil
			}),
		)
		model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{{
			{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "skip-1", ToolCallName: "skip", ToolCallArgsDelta: `{}`},
			{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 1, ToolCallID: "run-1", ToolCallName: "run", ToolCallArgsDelta: `{}`},
			{Type: aikit.StreamEventFinish, MessageID: "parallel", FinishReason: aikit.FinishReasonToolCalls},
		}}}
		built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
			return builder.Tools(set).ToolConcurrency(2).Hook(agent.HookFuncs{
				Name: "skip-policy",
				BeforeToolFunc: func(_ context.Context, _ agent.HookContext, call aikit.ToolCallInfo) (agent.ToolCallAction, error) {
					if call.Name == "skip" {
						return agent.ToolCallAction{Kind: agent.ToolCallSkip, Reason: "denied"}, nil
					}
					return agent.ToolCallAction{Kind: agent.ToolCallRun}, nil
				},
			})
		})
		result, err := built.Runner().Prompt("parallel").Run(context.Background())
		var maxTurns *agent.MaxTurnsError
		if !errors.As(err, &maxTurns) || result == nil || len(result.ToolResults) != 2 {
			t.Fatalf("Run() = (%#v, %v), want two partial tool results", result, err)
		}
		if skipExecutions.Load() != 0 || result.ToolResults[0].Output != "denied" || result.ToolResults[1].Output != `"ran"` {
			t.Fatalf("parallel results=%#v skip executions=%d", result.ToolResults, skipExecutions.Load())
		}
	})

	t.Run("invalid call repair", func(t *testing.T) {
		var executions atomic.Int32
		set := mustHookToolSet(t, hookDynamicTool(t, "actual", func(context.Context, json.RawMessage) (json.RawMessage, error) {
			executions.Add(1)
			return json.RawMessage(`{"repaired":true}`), nil
		}))
		model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{{
			{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "bad-1", ToolCallName: "typo", ToolCallArgsDelta: `{}`},
			{Type: aikit.StreamEventFinish, MessageID: "repair", FinishReason: aikit.FinishReasonToolCalls},
		}}}
		built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
			return builder.Tools(set).Hook(agent.HookFuncs{
				Name: "repair",
				InvalidToolCallFunc: func(_ context.Context, _ agent.HookContext, input aikit.RepairToolCallInput) (agent.InvalidToolCallAction, error) {
					repaired := input.ToolCall
					repaired.Name = "actual"
					return agent.InvalidToolCallAction{Kind: agent.InvalidToolCallRepair, Repaired: &repaired}, nil
				},
			})
		})
		result, err := built.Runner().Prompt("repair").Run(context.Background())
		var maxTurns *agent.MaxTurnsError
		if !errors.As(err, &maxTurns) || result == nil || len(result.ToolResults) != 1 {
			t.Fatalf("Run() = (%#v, %v), want repaired partial result", result, err)
		}
		if executions.Load() != 1 || result.ToolResults[0].Name != "actual" {
			t.Fatalf("repair result=%#v executions=%d", result.ToolResults, executions.Load())
		}
	})
}
