# Why ai-go

ai-go provides composable AI application primitives without making provider
configuration, agent orchestration, and UI streaming one inseparable layer.

## Provider-neutral workflows

High-level generation consumes small contracts from `llm` and shared values
from `aikit`. Provider packages own credentials, endpoints, wire formats, and
typed provider options. Applications can therefore keep the same completion or
agent workflow while selecting an appropriate provider model.

## Go-native extension points

The SDK uses concrete provider clients, small interfaces, and composition:

- concrete clients expose supported operations through their method sets;
- `provider.Client[P]` shares provider policy and HTTP behavior for provider
  implementations;
- `llm.Model`, `llm.EmbeddingModel`, and `llm.ImageModel` remain focused
  execution contracts; and
- `transport.Doer` keeps HTTP behavior injectable for tests and applications.

This retains the useful separation found in Rig's provider architecture while
using runtime validation and ordinary Go method sets instead of Rust typestate
or capability markers.

## Stream-first execution

Language models emit normalized events for text, reasoning, tool calls, usage,
sources, and generated files. Direct completions aggregate those events once;
agents build multi-step execution on the same stream; UI integrations translate
the resulting events at the boundary.

## Explicit ownership

Applications provide configuration explicitly. Provider clients own reusable
credentials and transports, requests own per-call options, and callers retain
control over contexts, cancellation, logging, and secret loading.

Continue with [Architecture](/docs/architecture) or
[Providers and clients](/core/providers-and-clients).
