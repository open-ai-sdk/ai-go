import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Providers","description":"","frontmatter":{},"headers":[],"relativePath":"providers/index.md","filePath":"providers/index.md"}');
const _sfc_main = { name: "providers/index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="providers" tabindex="-1">Providers <a class="header-anchor" href="#providers" aria-label="Permalink to &quot;Providers&quot;">​</a></h1><p>The <code>llm</code> contracts keep generation provider-neutral while each <code>provider/*</code> package owns a provider&#39;s configuration, API encoding, and typed options.</p><p>For providers with the client architecture, construct one concrete client and derive lightweight, operation-specific model handles from it. The client owns credentials, endpoints, and reusable HTTP resources; its method set shows which capabilities are implemented. The generic <code>provider.Client[P]</code> is reusable infrastructure for provider authors rather than the normal application API.</p><table tabindex="0"><thead><tr><th>Provider</th><th>Package</th><th>Main capability</th></tr></thead><tbody><tr><td>OpenAI</td><td><code>provider/openai</code></td><td>Responses API and Chat Completions</td></tr><tr><td>Anthropic</td><td><code>provider/anthropic</code></td><td>Language models</td></tr><tr><td>Gemini</td><td><code>provider/gemini</code></td><td>Language, embeddings, images, and native Gemini features</td></tr><tr><td>Kie</td><td><code>provider/kie</code></td><td>Image models</td></tr><tr><td>Compatible endpoints</td><td><code>provider/openaicompat</code></td><td>OpenAI-style Chat Completions</td></tr></tbody></table><p>Start with <a href="/providers/openai">OpenAI</a>, read the <a href="/core/providers-and-clients">providers and clients concept guide</a>, or see <a href="/providers/other-providers">other providers</a>. Provider options should normally be typed structs from that provider package. This catches unsupported or invalid options rather than silently ignoring them.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("providers/index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
