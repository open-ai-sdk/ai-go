import { ssrRenderAttrs, ssrRenderStyle } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Run the chat server","description":"","frontmatter":{},"headers":[],"relativePath":"guides/chat-server.md","filePath":"guides/chat-server.md"}');
const _sfc_main = { name: "guides/chat-server.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="run-the-chat-server" tabindex="-1">Run the chat server <a class="header-anchor" href="#run-the-chat-server" aria-label="Permalink to &quot;Run the chat server&quot;">​</a></h1><p>The repository includes a runnable server that exercises the production <code>aisdkhttp</code> boundary for text, tool, error, approval, and denial flows.</p><div class="language-sh vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang">sh</span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span style="${ssrRenderStyle({ "--shiki-light": "#6F42C1", "--shiki-dark": "#B392F0" })}">go</span><span style="${ssrRenderStyle({ "--shiki-light": "#032F62", "--shiki-dark": "#9ECBFF" })}"> run</span><span style="${ssrRenderStyle({ "--shiki-light": "#032F62", "--shiki-dark": "#9ECBFF" })}"> ./examples/chat-server</span></span>
<span class="line"><span style="${ssrRenderStyle({ "--shiki-light": "#6F42C1", "--shiki-dark": "#B392F0" })}">curl</span><span style="${ssrRenderStyle({ "--shiki-light": "#005CC5", "--shiki-dark": "#79B8FF" })}"> -i</span><span style="${ssrRenderStyle({ "--shiki-light": "#032F62", "--shiki-dark": "#9ECBFF" })}"> http://127.0.0.1:8787/healthz</span></span></code></pre></div><p>Use it as a reference for an AI SDK v7-compatible chat endpoint or to run the browser conformance suite locally. The server is deliberately small: your application supplies a model and run function; <code>aisdkhttp.Handler</code> handles request decoding, response headers, SSE chunks, flushing, and disconnect cancellation.</p><p>For protocol parameterization, <code>examples/multi-protocol</code> serves one Agent at <code>/ai-sdk</code> and <code>/ag-ui</code>. <code>aisdkhttp.HandlerFor</code> selects a protocol while the run function and <code>uistream.Pipe</code> driver stay shared.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("guides/chat-server.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const chatServer = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  chatServer as default
};
