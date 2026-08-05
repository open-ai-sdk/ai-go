import { resolveComponent, useSSRContext } from "vue";
import { ssrRenderAttrs, ssrRenderSuspense, ssrRenderComponent } from "vue/server-renderer";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Package map","description":"","frontmatter":{},"headers":[],"relativePath":"reference/package-map.md","filePath":"reference/package-map.md"}');
const _sfc_main = { name: "reference/package-map.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  const _component_Mermaid = resolveComponent("Mermaid");
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="package-map" tabindex="-1">Package map <a class="header-anchor" href="#package-map" aria-label="Permalink to &quot;Package map&quot;">​</a></h1><p>The dependency graph points downward. <code>aikit</code> is the leaf vocabulary; providers do not import the UI protocol, and <code>aisdk</code> does not import Agents or providers.</p><table tabindex="0"><thead><tr><th>Package</th><th>Responsibility</th></tr></thead><tbody><tr><td><code>ai</code></td><td>Non-Agent convenience operations such as objects, embeddings, images, and cost helpers</td></tr><tr><td><code>agent</code></td><td>Agent Builder, immutable Agent, per-run Runner, results, hooks, approvals, and multi-turn execution</td></tr><tr><td><code>aikit</code></td><td>Dependency-free messages, content, step/stream events, usage, warnings, and errors</td></tr><tr><td><code>llm</code></td><td>Model contracts, direct completion builder, normalized requests, provider options, embeddings, and images</td></tr><tr><td><code>tool</code></td><td>Typed/dynamic tools, canonical rich results, safe errors, immutable registry, and execution context</td></tr><tr><td><code>transport</code></td><td>Provider HTTP policy, retries, SSE reading, cancellation, and API error mapping</td></tr><tr><td><code>provider/*</code></td><td>Provider-specific constructors and wire codecs</td></tr><tr><td><code>uistream</code></td><td>Protocol-neutral request, frame, encoder/decoder/framer contracts, and event drain driver</td></tr><tr><td><code>uistream/ainode</code></td><td>AI SDK v7 wire implementation, imperative writer, persistence, and approvals</td></tr><tr><td><code>uistream/agui</code></td><td>Minimal AG-UI RunAgentInput and event-stream adapter</td></tr><tr><td><code>aisdk</code></td><td>Compatibility aliases and forwarders to <code>uistream/ainode</code></td></tr><tr><td><code>aisdkhttp</code></td><td>Protocol-parameterized HTTP/SSE boundary consuming one <code>aikit.StepEvent</code> iterator</td></tr><tr><td><code>mcp</code></td><td>MCP clients and dynamic tool integration</td></tr></tbody></table><p>Agent code follows one ownership path:</p>`);
  ssrRenderSuspense(_push, {
    default: () => {
      _push(ssrRenderComponent(_component_Mermaid, {
        id: "mermaid-127",
        class: "mermaid",
        graph: "flowchart%20LR%0A%20%20%20%20Builder%5B%22agent.Builder%22%5D%20--%3E%7C%22Build()%22%7C%20Agent%5B%22immutable%20*agent.Agent%22%5D%0A%20%20%20%20Agent%20--%3E%7C%22Runner()%22%7C%20Runner%5B%22per-invocation%20agent.Runner%22%5D%0A%20%20%20%20Runner%20--%3E%7C%22Run(ctx)%22%7C%20Result%5B%22*agent.Result%22%5D%0A%20%20%20%20Runner%20--%3E%7C%22Stream(ctx)%22%7C%20Events%5B%22iter.Seq2%5Baikit.StepEvent%2C%20error%5D%22%5D%0A"
      }, null, _parent));
    },
    fallback: () => {
      _push(` Loading... `);
    },
    _: 1
  });
  _push(`<p>Package <code>ai</code> does not alias or forward Agent symbols. The removed legacy Agent package has no compatibility replacement: build an <code>agent.Agent</code>, call <code>Agent.Runner()</code> for multi-turn execution, and use <code>llm.NewCompletion</code> for one direct model call. Applications that need to mock an Agent should define the narrow local interface their code consumes.</p><p>The conceptual documentation follows the same dependency order: direct <a href="/core/completions">Completions</a>, then <a href="/core/agents">Agents</a>, <a href="/core/agent-runner">Agent Runner</a>, <a href="/core/hooks">Hooks</a>, and <a href="/core/tools">Tools</a>.</p><p>The <a href="https://github.com/open-ai-sdk/ai-go#readme" target="_blank" rel="noreferrer">README</a> and Go package documentation remain the canonical API references. This site focuses on the workflows and architectural boundaries that make those APIs easier to compose.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("reference/package-map.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const packageMap = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  packageMap as default
};
