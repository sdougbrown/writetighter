import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'WriteTighter',
  description: 'A CLI that catches wordslop before it ships — deterministic lint plus contextual rewrites for Markdown.',
  base: '/',
  head: [
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
  ],
  themeConfig: {
    nav: [
      { text: 'Before / After', link: '/before-after' },
      { text: 'CLI Reference', link: '/cli' },
      { text: 'Agent Integration', link: '/agent-integration' },
      { text: 'GitHub', link: 'https://github.com/sdougbrown/writetighter' },
    ],
    sidebar: [
      {
        text: 'Start Here',
        items: [
          { text: 'Before / After', link: '/before-after' },
          { text: 'Why Tighter Prose', link: '/why' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI Reference', link: '/cli' },
          { text: 'Agent Integration', link: '/agent-integration' },
        ],
      },
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/sdougbrown/writetighter' },
    ],
    editLink: {
      pattern: 'https://github.com/sdougbrown/writetighter/edit/main/site/:path',
      text: 'Edit this page',
    },
  },
})
