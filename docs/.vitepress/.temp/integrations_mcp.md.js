import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Model Context Protocol","description":"","frontmatter":{},"headers":[],"relativePath":"integrations/mcp.md","filePath":"integrations/mcp.md"}');
const _sfc_main = { name: "integrations/mcp.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="model-context-protocol" tabindex="-1">Model Context Protocol <a class="header-anchor" href="#model-context-protocol" aria-label="Permalink to &quot;Model Context Protocol&quot;">​</a></h1><p>The <code>mcp</code> package initializes MCP clients, invokes remote tools, and adapts a one-shot discovery snapshot to the same immutable <code>tool.Set</code> used by native tools.</p><p>Use <code>mcp.ToolSetFromClientsContext(ctx, clients)</code> when discovery must respect a caller deadline. It follows every <code>nextCursor</code>, preserves server/page order (servers are sorted by name), and rejects cursor loops. The returned Set is a snapshot: later server list changes do not alter it, and notification-driven live refresh is intentionally deferred.</p><p>MCP text, images, structured content, resources, and future blocks retain their ordered typed representation. Unsupported variants remain raw structured content rather than placeholder strings. Response metadata and transport details are host-only. A remote <code>isError</code> uses server-supplied content as its safe model presentation while its operator cause remains available through normal Go error unwrapping.</p><p>Remote arguments must be an object, <code>null</code>, or empty; scalar and array arguments fail as <code>tool.InputError</code>. Native and MCP-discovered tools share normal hook, approval, context, and immutable-registry behavior.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("integrations/mcp.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const mcp = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  mcp as default
};
