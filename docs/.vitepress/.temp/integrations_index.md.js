import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Integrations","description":"","frontmatter":{},"headers":[],"relativePath":"integrations/index.md","filePath":"integrations/index.md"}');
const _sfc_main = { name: "integrations/index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="integrations" tabindex="-1">Integrations <a class="header-anchor" href="#integrations" aria-label="Permalink to &quot;Integrations&quot;">​</a></h1><p>Integrations connect ai-go&#39;s model and event contracts to external services and protocols.</p><h2 id="model-providers" tabindex="-1">Model providers <a class="header-anchor" href="#model-providers" aria-label="Permalink to &quot;Model providers&quot;">​</a></h2><p>Use concrete packages under <code>provider/*</code> for authentication, endpoints, wire formats, and typed provider options.</p><ul><li><a href="/providers/">Model provider overview</a></li><li><a href="/providers/openai">OpenAI</a></li><li><a href="/providers/other-providers">Other providers</a></li></ul><h2 id="protocol-integrations" tabindex="-1">Protocol integrations <a class="header-anchor" href="#protocol-integrations" aria-label="Permalink to &quot;Protocol integrations&quot;">​</a></h2><ul><li><p><a href="/integrations/mcp">Model Context Protocol</a> turns MCP capabilities into tools that the agent runtime can call.</p></li><li><p><a href="/integrations/ui-streams">AI SDK v7 UI streams</a> translates normalized events</p></li><li><p><a href="/integrations/protocol-extensions">UI stream protocol extensions</a> explains the event-driven protocol seam and AG-UI subset. into the browser-facing SSE protocol.</p></li></ul><p>For the provider/client mental model, start with <a href="/core/providers-and-clients">Providers and clients</a>.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("integrations/index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
