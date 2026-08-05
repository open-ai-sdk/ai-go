import { resolveComponent, useSSRContext } from "vue";
import { ssrRenderAttrs, ssrRenderSuspense, ssrRenderComponent } from "vue/server-renderer";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Architecture","description":"","frontmatter":{},"headers":[],"relativePath":"docs/architecture.md","filePath":"docs/architecture.md"}');
const _sfc_main = { name: "docs/architecture.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  const _component_Mermaid = resolveComponent("Mermaid");
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="architecture" tabindex="-1">Architecture <a class="header-anchor" href="#architecture" aria-label="Permalink to &quot;Architecture&quot;">​</a></h1><p>ai-go is organized as layers with a downward dependency direction. Provider code and UI protocols meet through shared model and event contracts rather than depending directly on one another.</p>`);
  ssrRenderSuspense(_push, {
    default: () => {
      _push(ssrRenderComponent(_component_Mermaid, {
        id: "mermaid-6",
        class: "mermaid",
        graph: "flowchart%20TD%0A%20%20%20%20Application%20--%3E%20AI%5B%22ai%20non-Agent%20conveniences%22%5D%0A%20%20%20%20Application%20--%3E%20Builder%5B%22agent.Builder%22%5D%0A%20%20%20%20Application%20--%3E%20Direct%5B%22llm%20direct%20completion%22%5D%0A%20%20%20%20Builder%20--%3E%20Agent%5B%22immutable%20agent.Agent%22%5D%0A%20%20%20%20Agent%20--%3E%20Runner%5B%22per-run%20agent.Runner%22%5D%0A%20%20%20%20Runner%20--%3E%20LLM%5B%22llm%20model%20contract%22%5D%0A%20%20%20%20Direct%20--%3E%20LLM%0A%20%20%20%20AI%20--%3E%20LLM%0A%0A%20%20%20%20Providers%5B%22provider%2F*%22%5D%20--%3E%20LLM%0A%20%20%20%20Providers%20--%3E%20Transport%5B%22transport%22%5D%0A%0A%20%20%20%20LLM%20--%3E%20Aikit%5B%22aikit%20shared%20vocabulary%22%5D%0A%20%20%20%20Transport%20--%3E%20Aikit%0A%20%20%20%20Runner%20--%3E%20Tools%5B%22immutable%20tool.Set%22%5D%0A%20%20%20%20MCP%5B%22mcp%22%5D%20--%3E%20Tools%0A%20%20%20%20Application%20--%3E%20HTTP%5B%22aisdkhttp%22%5D%0A%20%20%20%20HTTP%20--%3E%20Stream%5B%22uistream%22%5D%0A%20%20%20%20HTTP%20--%3E%20AINode%5B%22uistream%2Fainode%22%5D%0A%20%20%20%20HTTP%20--%3E%20AGUI%5B%22uistream%2Fagui%22%5D%0A%20%20%20%20AINode%20--%3E%20Stream%0A%20%20%20%20AGUI%20--%3E%20Stream%0A%20%20%20%20Stream%20--%3E%20Aikit%0A%20%20%20%20Compat%5B%22aisdk%20compatibility%22%5D%20--%3E%20AINode%0A"
      }, null, _parent));
    },
    fallback: () => {
      _push(` Loading... `);
    },
    _: 1
  });
  _push(`<h2 id="public-layers" tabindex="-1">Public layers <a class="header-anchor" href="#public-layers" aria-label="Permalink to &quot;Public layers&quot;">​</a></h2><h3 id="application-facade" tabindex="-1">Application facade <a class="header-anchor" href="#application-facade" aria-label="Permalink to &quot;Application facade&quot;">​</a></h3><p>The <code>ai</code> package contains non-Agent convenience operations. Agent contracts are not aliased or forwarded there; applications import their canonical owners directly.</p><h3 id="completion-and-agent-execution" tabindex="-1">Completion and agent execution <a class="header-anchor" href="#completion-and-agent-execution" aria-label="Permalink to &quot;Completion and agent execution&quot;">​</a></h3><p><code>llm</code> owns direct model contracts and completion request builders. <code>agent</code> owns one public multi-turn lifecycle: a value Builder creates an immutable Agent, and each value Runner owns one invocation&#39;s ordered messages and overrides. <code>Run</code> and <code>Stream</code> share one driver and Result reducer.</p><p>This ordering is reflected in the concept guides: first configure an <a href="/core/agents">Agent</a>, then create a Runner and execute through the <a href="/core/agent-runner">Agent Runner</a> lifecycle.</p><h3 id="shared-vocabulary" tabindex="-1">Shared vocabulary <a class="header-anchor" href="#shared-vocabulary" aria-label="Permalink to &quot;Shared vocabulary&quot;">​</a></h3><p><code>aikit</code> contains dependency-light messages, content parts, stream events, usage, warnings, and errors. Providers and consumers exchange these values without importing one another.</p><h3 id="providers-and-transport" tabindex="-1">Providers and transport <a class="header-anchor" href="#providers-and-transport" aria-label="Permalink to &quot;Providers and transport&quot;">​</a></h3><p>Concrete packages under <code>provider/*</code> encode provider APIs and typed options. Provider clients share credentials and HTTP resources; model handles add a model ID and operation. <code>transport</code> centralizes safe request construction, SSE handling, cancellation, and provider HTTP errors.</p><h3 id="integrations" tabindex="-1">Integrations <a class="header-anchor" href="#integrations" aria-label="Permalink to &quot;Integrations&quot;">​</a></h3><p><code>mcp</code> adapts Model Context Protocol tools into the immutable registry. <code>aisdkhttp</code> consumes the Runner&#39;s single-owner <code>aikit.StepEvent</code> iterator through <code>uistream.Pipe</code>. Protocol adapters translate leaf events without importing <code>agent</code>; <code>aisdk</code> preserves the original AI SDK v7 public surface as aliases and forwarders to <code>uistream/ainode</code>.</p><p>See the <a href="/reference/package-map">package map</a> for package-by-package ownership.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("docs/architecture.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const architecture = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  architecture as default
};
