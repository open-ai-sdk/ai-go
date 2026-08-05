import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Examples","description":"","frontmatter":{},"headers":[],"relativePath":"examples/index.md","filePath":"examples/index.md"}');
const _sfc_main = { name: "examples/index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="examples" tabindex="-1">Examples <a class="header-anchor" href="#examples" aria-label="Permalink to &quot;Examples&quot;">​</a></h1><p>Runnable examples live in the repository&#39;s <code>examples</code> directory. This section will grow by provider and use case as more examples are added.</p><h2 id="chat-server" tabindex="-1">Chat server <a class="header-anchor" href="#chat-server" aria-label="Permalink to &quot;Chat server&quot;">​</a></h2><p>The current chat-server example is an AI SDK v7 conformance fixture. It demonstrates the HTTP/SSE boundary and representative text, tool, approval, and error event scenarios; it does not execute a real provider tool loop.</p><ul><li><a href="/guides/chat-server">Chat server walkthrough</a></li><li><a href="https://github.com/open-ai-sdk/ai-go/tree/main/examples/chat-server" target="_blank" rel="noreferrer">Source code</a></li></ul><p>For smaller API-focused snippets, start with the <a href="/getting-started">Quickstart</a> and <a href="/core/">Concepts</a>.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("examples/index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
