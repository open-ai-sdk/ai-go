import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Why ai-go","description":"","frontmatter":{},"headers":[],"relativePath":"docs/why-ai-go.md","filePath":"docs/why-ai-go.md"}');
const _sfc_main = { name: "docs/why-ai-go.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="why-ai-go" tabindex="-1">Why ai-go <a class="header-anchor" href="#why-ai-go" aria-label="Permalink to &quot;Why ai-go&quot;">​</a></h1><p>ai-go provides composable AI application primitives without making provider configuration, agent orchestration, and UI streaming one inseparable layer.</p><h2 id="provider-neutral-workflows" tabindex="-1">Provider-neutral workflows <a class="header-anchor" href="#provider-neutral-workflows" aria-label="Permalink to &quot;Provider-neutral workflows&quot;">​</a></h2><p>High-level generation consumes small contracts from <code>llm</code> and shared values from <code>aikit</code>. Provider packages own credentials, endpoints, wire formats, and typed provider options. Applications can therefore keep the same completion or agent workflow while selecting an appropriate provider model.</p><h2 id="go-native-extension-points" tabindex="-1">Go-native extension points <a class="header-anchor" href="#go-native-extension-points" aria-label="Permalink to &quot;Go-native extension points&quot;">​</a></h2><p>The SDK uses concrete provider clients, small interfaces, and composition:</p><ul><li>concrete clients expose supported operations through their method sets;</li><li><code>provider.Client[P]</code> shares provider policy and HTTP behavior for provider implementations;</li><li><code>llm.Model</code>, <code>llm.EmbeddingModel</code>, and <code>llm.ImageModel</code> remain focused execution contracts; and</li><li><code>transport.Doer</code> keeps HTTP behavior injectable for tests and applications.</li></ul><p>This separation uses runtime validation and ordinary Go method sets to keep provider capabilities explicit without coupling applications to provider configuration details.</p><h2 id="stream-first-execution" tabindex="-1">Stream-first execution <a class="header-anchor" href="#stream-first-execution" aria-label="Permalink to &quot;Stream-first execution&quot;">​</a></h2><p>Language models emit normalized events for text, reasoning, tool calls, usage, sources, and generated files. Direct completions aggregate those events once; agents build multi-step execution on the same stream; UI integrations translate the resulting events at the boundary.</p><h2 id="explicit-ownership" tabindex="-1">Explicit ownership <a class="header-anchor" href="#explicit-ownership" aria-label="Permalink to &quot;Explicit ownership&quot;">​</a></h2><p>Applications provide configuration explicitly. Provider clients own reusable credentials and transports, requests own per-call options, and callers retain control over contexts, cancellation, logging, and secret loading.</p><p>Continue with <a href="/docs/architecture">Architecture</a> or <a href="/core/providers-and-clients">Providers and clients</a>.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("docs/why-ai-go.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const whyAiGo = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  whyAiGo as default
};
