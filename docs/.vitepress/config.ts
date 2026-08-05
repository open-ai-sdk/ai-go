import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'

const repository = 'https://github.com/open-ai-sdk/ai-go'

const docsSidebar = [
  {
    text: 'Documentation',
    items: [
      { text: 'Overview', link: '/docs/' },
      { text: 'Quickstart', link: '/getting-started' },
      { text: 'Why ai-go', link: '/docs/why-ai-go' },
      { text: 'Architecture', link: '/docs/architecture' },
    ],
  },
  {
    text: 'Concepts',
    collapsed: false,
    items: [
      { text: 'Overview', link: '/core/' },
      { text: 'Providers and clients', link: '/core/providers-and-clients' },
      { text: 'Completions', link: '/core/completions' },
      { text: 'Messages and content', link: '/core/messages-and-content' },
      { text: 'Agents', link: '/core/agents' },
      { text: 'Agent Runner', link: '/core/agent-runner' },
      { text: 'Hooks', link: '/core/hooks' },
      { text: 'Tools', link: '/core/tools' },
      { text: 'Streaming', link: '/core/streaming' },
      { text: 'Structured output', link: '/core/structured-output' },
      { text: 'Embeddings', link: '/core/embeddings' },
      { text: 'Media generation', link: '/core/media-generation' },
      { text: 'Observability', link: '/core/observability' },
    ],
  },
  {
    text: 'Integrations',
    collapsed: false,
    items: [
      { text: 'Overview', link: '/integrations/' },
      {
        text: 'Model providers',
        collapsed: false,
        items: [
          { text: 'Overview', link: '/providers/' },
          { text: 'OpenAI', link: '/providers/openai' },
          { text: 'Other providers', link: '/providers/other-providers' },
        ],
      },
      { text: 'Model Context Protocol', link: '/integrations/mcp' },
      { text: 'AI SDK v7 UI streams', link: '/integrations/ui-streams' },
      { text: 'AG-UI and TanStack AI', link: '/integrations/ag-ui' },
      { text: 'Protocol extensions', link: '/integrations/protocol-extensions' },
    ],
  },
  {
    text: 'Extensions',
    items: [{ text: 'Extend ai-go', link: '/docs/extensions' }],
  },
]

const guidesSidebar = [
  {
    text: 'Tutorials & Guides',
    items: [
      { text: 'Overview', link: '/guides/' },
      { text: 'Build a chat server', link: '/guides/chat-server' },
      { text: 'Error handling', link: '/guides/error-handling' },
    ],
  },
]

const examplesSidebar = [
  {
    text: 'Examples',
    items: [{ text: 'Overview', link: '/examples/' }],
  },
]

const referenceSidebar = [
  {
    text: 'Reference',
    items: [
      { text: 'Overview', link: '/reference/' },
      { text: 'Package map', link: '/reference/package-map' },
    ],
  },
]

export default withMermaid(defineConfig({
  title: 'ai-go',
  description: 'A provider-neutral Go SDK for building AI applications.',
  base: process.env.GITHUB_ACTIONS ? '/ai-go/' : '/',
  cleanUrls: true,
  head: [
    ['meta', { name: 'theme-color', content: '#0f766e' }],
    ['meta', { name: 'author', content: 'open-ai-sdk' }],
  ],
  themeConfig: {
    logo: { light: '/logo-light.svg', dark: '/logo-dark.svg', alt: 'ai-go' },
    nav: [
      { text: 'Get Started', link: '/getting-started' },
      { text: 'Docs', link: '/docs/' },
      { text: 'Tutorials & Guides', link: '/guides/' },
      { text: 'Examples', link: '/examples/' },
      { text: 'API Reference', link: 'https://pkg.go.dev/github.com/open-ai-sdk/ai-go' },
    ],
    sidebar: {
      '/guides/': guidesSidebar,
      '/examples/': examplesSidebar,
      '/reference/': referenceSidebar,
      '/': docsSidebar,
    },
    socialLinks: [{ icon: 'github', link: repository }],
    editLink: { pattern: `${repository}/edit/main/docs/:path`, text: 'Edit this page on GitHub' },
    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © open-ai-sdk contributors',
    },
    search: { provider: 'local' },
    outline: { level: [2, 3], label: 'On this page' },
  },
}))
