import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Concepts","description":"","frontmatter":{},"headers":[],"relativePath":"core/index.md","filePath":"core/index.md"}');
const _sfc_main = { name: "core/index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="concepts" tabindex="-1">Concepts <a class="header-anchor" href="#concepts" aria-label="Permalink to &quot;Concepts&quot;">​</a></h1><p>Concept pages explain the stable abstractions used across providers and application workflows. They are organized by SDK capability rather than by individual helper function.</p><ul><li><a href="/core/providers-and-clients">Providers and clients</a> — provider-wide state, model handles, capabilities, and mixed-modality output.</li><li><a href="/core/completions">Completions</a> — provider-neutral language-model requests and responses.</li><li><a href="/core/agents">Agents</a> — reusable immutable model, tool, and policy defaults.</li><li><a href="/core/agent-runner">Agent Runner</a> — ordered input, per-run overrides, multi-turn execution and results.</li><li><a href="/core/hooks">Hooks</a> — run-local lifecycle policy, model-turn retries, streaming observations, and result presentation.</li><li><a href="/core/tools">Tools</a> — typed tools, ordered rich results, safe errors, invocation context, and approvals.</li><li><a href="/core/streaming">Streaming</a> — normalized model and Agent Runner events.</li><li><a href="/core/structured-output">Structured output</a> — schema-backed Go values.</li><li><a href="/core/embeddings">Embeddings</a> — single and batched vector generation.</li><li><a href="/core/media-generation">Media generation</a> — image models, inputs, and generated media.</li><li><a href="/core/observability">Observability</a> — provider-neutral tracing and content recording controls.</li></ul><p>Provider setup and protocol adapters belong under <a href="/integrations/">Integrations</a>. End-to-end application workflows belong under <a href="/guides/">Tutorials &amp; Guides</a>, and identifier-level details belong in the <a href="https://pkg.go.dev/github.com/open-ai-sdk/ai-go" target="_blank" rel="noreferrer">API reference</a>.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("core/index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
