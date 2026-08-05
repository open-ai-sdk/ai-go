import { ssrRenderAttrs } from "vue/server-renderer";
import { useSSRContext } from "vue";
import { _ as _export_sfc } from "./plugin-vue_export-helper.1tPrXgE0.js";
const __pageData = JSON.parse('{"title":"Documentation","description":"","frontmatter":{},"headers":[],"relativePath":"docs/index.md","filePath":"docs/index.md"}');
const _sfc_main = { name: "docs/index.md" };
function _sfc_ssrRender(_ctx, _push, _parent, _attrs, $props, $setup, $data, $options) {
  _push(`<div${ssrRenderAttrs(_attrs)}><h1 id="documentation" tabindex="-1">Documentation <a class="header-anchor" href="#documentation" aria-label="Permalink to &quot;Documentation&quot;">​</a></h1><p>ai-go separates provider integrations, model contracts, agent workflows, and transport protocols into focused Go packages. Start with the quickstart, then use concepts to understand the abstractions and integrations to configure a specific service.</p><h2 id="quickstart" tabindex="-1">Quickstart <a class="header-anchor" href="#quickstart" aria-label="Permalink to &quot;Quickstart&quot;">​</a></h2><ul><li><a href="/getting-started">Get started</a> — install ai-go, create a provider client, and generate text.</li><li><a href="/core/agents">Agents</a> — configure reusable immutable model, tool, and policy defaults.</li><li><a href="/core/agent-runner">Agent Runner</a> — add ordered input and request-local overrides, then execute or stream one invocation.</li><li><a href="/core/hooks">Hooks</a> — add run-local lifecycle observation, policy, model-turn retry, and streaming delta handling.</li><li><a href="/core/tools">Tools</a> — define schema-backed tools with rich output, safe failures, and request context.</li></ul><h2 id="understand-the-architecture" tabindex="-1">Understand the architecture <a class="header-anchor" href="#understand-the-architecture" aria-label="Permalink to &quot;Understand the architecture&quot;">​</a></h2><ul><li><a href="/docs/why-ai-go">Why ai-go</a> explains the design goals and package boundaries.</li><li><a href="/docs/architecture">Architecture</a> maps the public layers and dependency direction.</li><li><a href="/core/">Concepts</a> covers providers, completions, messages, Agents, Agent Runner, Hooks, Tools, streaming, and structured output in dependency order.</li></ul><h2 id="connect-services" tabindex="-1">Connect services <a class="header-anchor" href="#connect-services" aria-label="Permalink to &quot;Connect services&quot;">​</a></h2><ul><li><a href="/integrations/">Integrations</a> covers model providers, MCP, and AI SDK UI streams.</li><li><a href="/docs/extensions">Extend ai-go</a> explains the existing extension seams for custom models and OpenAI-compatible services.</li></ul><p>For runnable walkthroughs, continue to <a href="/guides/">Tutorials &amp; Guides</a> or browse the <a href="/examples/">Examples</a>. Exact exported identifiers live in the <a href="https://pkg.go.dev/github.com/open-ai-sdk/ai-go" target="_blank" rel="noreferrer">Go API reference</a>.</p></div>`);
}
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("docs/index.md");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};
const index = /* @__PURE__ */ _export_sfc(_sfc_main, [["ssrRender", _sfc_ssrRender]]);
export {
  __pageData,
  index as default
};
