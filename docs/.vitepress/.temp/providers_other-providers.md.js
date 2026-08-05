import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Other providers","description":"","frontmatter":{},"headers":[],"relativePath":"providers/other-providers.md","filePath":"providers/other-providers.md"}');
const _sfc_main = { name: "providers/other-providers.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="other-providers" tabindex="-1">Other providers <a class="header-anchor" href="#other-providers" aria-label="Permalink to &quot;Other providers&quot;">​</a></h1><p><code>ai-go</code> ships focused provider packages rather than a global provider registry.</p><ul><li><strong>Anthropic:</strong> create a language model with <code>anthropic.NewLanguageModel</code>, or create an <code>anthropic.Provider</code> and select its model.</li><li><strong>Gemini:</strong> <code>gemini.NewLanguageModel</code> provides the compatible language-model path. The package also includes native language models plus embedding and image model constructors for Gemini-specific capabilities.</li><li><strong>Kie:</strong> <code>kie.NewProvider(apiKey).Image(modelID)</code> creates an image model. The provider can also read <code>KIE_API_KEY</code> when no key is supplied.</li><li><strong>OpenAI-compatible APIs:</strong> use <code>openaicompat.NewModel</code> with its explicit <code>Config</code>, including a compatibility provider, model ID, and endpoint/auth configuration.</li></ul><p>All models are used through the same <code>ai</code> facade. Select a model type that matches the operation: language models for text and objects, embedding models for <code>Embed</code>, and image models for image generation.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("providers/other-providers.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const otherProviders = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  otherProviders as default
};
