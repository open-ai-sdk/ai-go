import { ssrRenderAttrs, ssrRenderStyle } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"","description":"","frontmatter":{"layout":"home","hero":{"name":"ai-go","text":"AI application primitives for Go","tagline":"Provider-neutral generation, typed tools, multi-step agents, and AI SDK v7 UI streams.","actions":[{"theme":"brand","text":"Get started","link":"/getting-started"},{"theme":"alt","text":"Read the docs","link":"/docs/"}]},"features":[{"icon":"⚡","title":"Focused Go APIs","details":"Use llm for direct model calls, then build an immutable Agent and create a fresh Runner for each invocation."},{"icon":"🧰","title":"Typed tools and agents","details":"Build schema-validated Go tools and let the runtime execute multi-step tool loops."},{"icon":"🔌","title":"Provider-neutral by design","details":"Use OpenAI, Anthropic, Gemini, Kie, or OpenAI-compatible models behind focused contracts."},{"icon":"🌊","title":"UI-stream ready","details":"Produce AI SDK v7-compatible SSE streams from a small net/http boundary."}]},"headers":[],"relativePath":"index.md","filePath":"index.md"}');
const _sfc_main = { name: "index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h2 id="start-with-a-model" tabindex="-1">Start with a model <a class="header-anchor" href="#start-with-a-model" aria-label="Permalink to &quot;Start with a model&quot;">​</a></h2><p>Install the module, configure a provider, then use <code>llm.NewCompletion</code> for one model call or <code>agent.New(...).Build()</code> for reusable multi-turn execution. Canonical packages keep contract ownership explicit without Agent aliases or compatibility shims.</p><p>Read <a href="/core/agents">Agents</a> before <a href="/core/agent-runner">Agent Runner</a>: the first defines reusable configuration, while the second owns input, overrides, and execution.</p><div class="language-sh vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang">sh</span><pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0"><code><span class="line"><span style="${ssrRenderStyle({ "--shiki-light": "#6F42C1", "--shiki-dark": "#B392F0" })}">go</span><span style="${ssrRenderStyle({ "--shiki-light": "#032F62", "--shiki-dark": "#9ECBFF" })}"> get</span><span style="${ssrRenderStyle({ "--shiki-light": "#032F62", "--shiki-dark": "#9ECBFF" })}"> github.com/open-ai-sdk/ai-go</span></span></code></pre></div><p><a href="/getting-started">Read the quickstart →</a> · <a href="/docs/">Browse the documentation →</a></p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
