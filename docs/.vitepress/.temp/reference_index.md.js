import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Reference","description":"","frontmatter":{},"headers":[],"relativePath":"reference/index.md","filePath":"reference/index.md"}');
const _sfc_main = { name: "reference/index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="reference" tabindex="-1">Reference <a class="header-anchor" href="#reference" aria-label="Permalink to &quot;Reference&quot;">​</a></h1><ul><li><a href="/reference/package-map">Package map</a> explains ownership and dependency direction inside the repository.</li><li><a href="https://pkg.go.dev/github.com/open-ai-sdk/ai-go" target="_blank" rel="noreferrer">Go API reference</a> lists the exported packages, types, functions, and methods.</li><li><a href="https://github.com/open-ai-sdk/ai-go" target="_blank" rel="noreferrer">Source repository</a> contains examples, tests, and release history.</li></ul><p>This site focuses on concepts and workflows. Use <code>pkg.go.dev</code> as the canonical identifier-level API reference.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("reference/index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
