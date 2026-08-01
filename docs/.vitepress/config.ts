import { defineConfig } from 'vitepress'

const repository = 'https://github.com/open-ai-sdk/ai-go'

export default defineConfig({
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
      { text: 'Get started', link: '/getting-started' },
      { text: 'Core concepts', link: '/core/generate-text' },
      { text: 'Integrations', link: '/integrations/ui-streams' },
      { text: 'Reference', link: '/reference/package-map' },
    ],
    sidebar: {
      '/core/': [
        { text: 'Core concepts', items: [
          { text: 'Generate text', link: '/core/generate-text' },
          { text: 'Direct completions', link: '/core/completions' },
          { text: 'Providers and clients', link: '/core/providers-and-clients' },
          { text: 'Stream responses', link: '/core/streaming' },
          { text: 'Structured output', link: '/core/structured-output' },
          { text: 'Typed tools', link: '/core/tools' },
        ] },
      ],
      '/providers/': [
        { text: 'Providers', items: [
          { text: 'Overview', link: '/providers/' },
          { text: 'OpenAI', link: '/providers/openai' },
          { text: 'Other providers', link: '/providers/other-providers' },
        ] },
      ],
      '/integrations/': [
        { text: 'Integrations', items: [
          { text: 'AI SDK v7 UI streams', link: '/integrations/ui-streams' },
          { text: 'Model Context Protocol', link: '/integrations/mcp' },
        ] },
      ],
      '/guides/': [
        { text: 'Guides', items: [
          { text: 'Chat server', link: '/guides/chat-server' },
          { text: 'Observability', link: '/guides/observability' },
        ] },
      ],
      '/reference/': [
        { text: 'Reference', items: [
          { text: 'Package map', link: '/reference/package-map' },
        ] },
      ],
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
})
