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
      { text: 'Plugins', link: '/claude-plugin' },
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
      {
        text: 'Plugins',
        items: [
          { text: 'Claude Code', link: '/claude-plugin' },
          { text: 'Codex', link: '/codex-plugin' },
        ],
      },
      {
        text: 'Examples',
        items: [
          { text: 'Project config', link: 'https://github.com/sdougbrown/writetighter/blob/main/examples/writetighter.toml' },
          { text: 'User config', link: 'https://github.com/sdougbrown/writetighter/blob/main/examples/user-config.toml' },
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
