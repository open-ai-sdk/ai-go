import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Tutorials & Guides","description":"","frontmatter":{},"headers":[],"relativePath":"guides/index.md","filePath":"guides/index.md"}');
const _sfc_main = { name: "guides/index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="tutorials-guides" tabindex="-1">Tutorials &amp; Guides <a class="header-anchor" href="#tutorials-guides" aria-label="Permalink to &quot;Tutorials &amp; Guides&quot;">​</a></h1><p>Guides combine multiple ai-go concepts into end-to-end application workflows.</p><ul><li><a href="/guides/chat-server">Build a chat server</a> — connect generation to an HTTP chat endpoint and UI stream.</li><li><a href="/guides/error-handling">Error handling</a> — classify completion, prompt, tool, and structured-output failures with partial-result recovery.</li></ul><p>Use <a href="/core/">Concepts</a> for individual abstractions and <a href="/examples/">Examples</a> for the runnable programs maintained in the repository.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("guides/index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
