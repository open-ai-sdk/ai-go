import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Extend ai-go","description":"","frontmatter":{},"headers":[],"relativePath":"docs/extensions.md","filePath":"docs/extensions.md"}');
const _sfc_main = { name: "docs/extensions.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="extend-ai-go" tabindex="-1">Extend ai-go <a class="header-anchor" href="#extend-ai-go" aria-label="Permalink to &quot;Extend ai-go&quot;">​</a></h1><p>ai-go exposes extension points at different layers. Choose the smallest one that matches the integration.</p><h2 id="implement-a-model-contract" tabindex="-1">Implement a model contract <a class="header-anchor" href="#implement-a-model-contract" aria-label="Permalink to &quot;Implement a model contract&quot;">​</a></h2><p>Implement <code>llm.Model</code> for a language model, <code>llm.EmbeddingModel</code> for embeddings, or <code>llm.ImageModel</code> for image generation. These interfaces are intentionally small so external packages can implement and test them without depending on an agent runtime.</p><h2 id="build-a-provider-client" tabindex="-1">Build a provider client <a class="header-anchor" href="#build-a-provider-client" aria-label="Permalink to &quot;Build a provider client&quot;">​</a></h2><p>Provider authors can compose <code>provider.Client[P]</code> with a policy that supplies a provider name, base URL, and request authorization. The concrete provider package should then expose a concrete Client whose method set contains only the operations it implements.</p><p>Read <a href="/core/providers-and-clients">Providers and clients</a> for the ownership and capability model.</p><h2 id="adapt-an-openai-compatible-endpoint" tabindex="-1">Adapt an OpenAI-compatible endpoint <a class="header-anchor" href="#adapt-an-openai-compatible-endpoint" aria-label="Permalink to &quot;Adapt an OpenAI-compatible endpoint&quot;">​</a></h2><p>The <code>provider/openaicompat</code> package provides the Chat Completions protocol plus optional hooks for provider naming, capability flags, tool sanitization, request rewriting, and response decoding. Use it when the remote API follows the OpenAI-compatible wire shape; implement a native model when it does not.</p><p>See the <a href="/providers/">provider integrations</a> and the <a href="https://pkg.go.dev/github.com/open-ai-sdk/ai-go/provider/openaicompat" target="_blank" rel="noreferrer">Go API reference</a> for the exact contracts.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("docs/extensions.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const extensions = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  extensions as default
};
