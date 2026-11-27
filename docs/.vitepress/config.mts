import { defineConfig, type DefaultTheme } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'

function englishNav(): DefaultTheme.NavItem[] {
  return [
    { text: 'Getting Started', link: '/getting-started/overview', activeMatch: '/getting-started/' },
    { text: 'User Guide', link: '/user-guide/installation', activeMatch: '/user-guide/' },
    { text: 'Self-Hosting', link: '/deployment/docker-compose', activeMatch: '/deployment/' },
    { text: 'Administration', link: '/administration/providers-and-models', activeMatch: '/administration/|/security/' },
    { text: 'Reference', link: '/reference/cli-and-state', activeMatch: '/reference/|/contributing/' },
    { text: 'Try Online', link: 'https://aistrun.cc' }
  ]
}

function chineseNav(): DefaultTheme.NavItem[] {
  return [
    { text: '快速开始', link: '/zh/getting-started/overview', activeMatch: '/zh/getting-started/' },
    { text: '用户指南', link: '/zh/user-guide/installation', activeMatch: '/zh/user-guide/' },
    { text: '私有部署', link: '/zh/deployment/docker-compose', activeMatch: '/zh/deployment/' },
    { text: '管控治理', link: '/zh/administration/providers-and-models', activeMatch: '/zh/(administration|security)/' },
    { text: '参考手册', link: '/zh/reference/cli-and-state', activeMatch: '/zh/(reference|contributing)/' },
    { text: '在线体验', link: 'https://aistrun.cc' }
  ]
}

function englishSidebar(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: 'Getting Started',
      items: [
        { text: 'Overview & Architecture', link: '/getting-started/overview' },
        { text: 'Quickstart (Public Trial)', link: '/getting-started/quickstart' },
        { text: '5-Minute Self-Hosting', link: '/getting-started/self-hosting' }
      ]
    },
    {
      text: 'User Guide',
      items: [
        { text: 'Install, Upgrade & Language', link: '/user-guide/installation' },
        { text: 'Login & Account Registration', link: '/user-guide/authentication' },
        { text: 'Interactive, One-Shot & Pipe', link: '/user-guide/usage-modes' },
        { text: 'Multi-Host SSH & File Edit', link: '/user-guide/ssh-and-file-edit' },
        { text: 'Sessions & Environment Memory', link: '/user-guide/sessions-and-memory' }
      ]
    },
    {
      text: 'Deployment & Operations',
      items: [
        { text: 'Docker Compose Deployment', link: '/deployment/docker-compose' },
        { text: 'HTTPS Reverse Proxy & Access', link: '/deployment/reverse-proxy' },
        { text: 'Air-Gapped & Local LLMs', link: '/deployment/air-gapped-and-local-llm' },
        { text: 'Operations, Backup & Fly.io', link: '/deployment/operations' }
      ]
    },
    {
      text: 'Administration & Governance',
      items: [
        { text: 'Providers, Priority & Cooldown', link: '/administration/providers-and-models' },
        { text: 'Users, Invite Codes & Quotas', link: '/administration/users-and-quotas' },
        { text: 'Command Blacklist & TUI Guard', link: '/administration/command-guard' },
        { text: 'Tool & Traffic Audit Logs', link: '/administration/audit-logs' }
      ]
    },
    {
      text: 'Security',
      items: [
        { text: 'Security Architecture', link: '/security/overview' }
      ]
    },
    {
      text: 'Reference',
      items: [
        { text: 'CLI & Local State (~/.aist)', link: '/reference/cli-and-state' },
        { text: 'Configuration & Env Vars', link: '/reference/configuration' },
        { text: 'HTTP API & Error Codes', link: '/reference/api-and-errors' }
      ]
    },
    {
      text: 'Contributing & Internals',
      items: [
        { text: 'Local Development & Testing', link: '/contributing/development' },
        { text: 'Internal Design Deep Dive', link: '/contributing/architecture' },
        { text: 'Naming Conventions', link: '/contributing/naming' }
      ]
    }
  ]
}

function chineseSidebar(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: '快速开始',
      items: [
        { text: '产品定位与核心架构', link: '/zh/getting-started/overview' },
        { text: '快速体验（官方服务）', link: '/zh/getting-started/quickstart' },
        { text: '5 分钟私有化部署', link: '/zh/getting-started/self-hosting' }
      ]
    },
    {
      text: '用户指南',
      items: [
        { text: '安装、升级、卸载与语言切换', link: '/zh/user-guide/installation' },
        { text: '登录认证与邀请码注册', link: '/zh/user-guide/authentication' },
        { text: '交互对话、单次提问与管道分析', link: '/zh/user-guide/usage-modes' },
        { text: '多主机 SSH 与安全文件编辑', link: '/zh/user-guide/ssh-and-file-edit' },
        { text: '会话恢复与本机环境记忆', link: '/zh/user-guide/sessions-and-memory' }
      ]
    },
    {
      text: '部署与运维',
      items: [
        { text: 'Docker Compose 生产部署', link: '/zh/deployment/docker-compose' },
        { text: 'HTTPS 反向代理与后台防护', link: '/zh/deployment/reverse-proxy' },
        { text: '纯内网离线部署与本地模型接入', link: '/zh/deployment/air-gapped-and-local-llm' },
        { text: '健康检查、备份升级与 Fly.io', link: '/zh/deployment/operations' }
      ]
    },
    {
      text: '管控与治理',
      items: [
        { text: '模型路由、优先级与自动冷却', link: '/zh/administration/providers-and-models' },
        { text: '用户管理、邀请码与周 Token 限额', link: '/zh/administration/users-and-quotas' },
        { text: '命令黑名单与全屏程序拦截', link: '/zh/administration/command-guard' },
        { text: '工具调用与请求审计日志', link: '/zh/administration/audit-logs' }
      ]
    },
    {
      text: '安全架构',
      items: [
        { text: '数据流向、密钥托管与信任边界', link: '/zh/security/overview' }
      ]
    },
    {
      text: '参考手册',
      items: [
        { text: 'CLI 命令与状态目录 (~/.aist)', link: '/zh/reference/cli-and-state' },
        { text: '配置项与环境变量总表', link: '/zh/reference/configuration' },
        { text: 'HTTP 接口契约与错误码', link: '/zh/reference/api-and-errors' }
      ]
    },
    {
      text: '参与贡献与内部设计',
      items: [
        { text: '本地开发、构建与测试', link: '/zh/contributing/development' },
        { text: '核心模块设计详解', link: '/zh/contributing/architecture' },
        { text: '代码与领域命名规范', link: '/zh/contributing/naming' }
      ]
    }
  ]
}

export default withMermaid(
  defineConfig({
    title: 'AIST',
    description: 'On-Call AI Shell assistanT for Server Operations & Troubleshooting',
    cleanUrls: true,
    vite: {
      optimizeDeps: {
        include: ['mermaid', 'fastdom']
      }
    },
    head: [
      ['link', { rel: 'icon', type: 'image/svg+xml', href: '/logo.svg' }]
    ],
    themeConfig: {
      logo: {
        light: '/logo.svg',
        dark: '/logo-bw.svg',
        alt: 'AIST'
      },
      socialLinks: [
        { icon: 'github', link: 'https://github.com/llaoj/aist' }
      ],
      search: {
        provider: 'local',
        options: {
          locales: {
            zh: {
              translations: {
                button: {
                  buttonText: '搜索文档',
                  buttonAriaLabel: '搜索文档'
                },
                modal: {
                  noResultsText: '无法找到相关结果',
                  resetButtonTitle: '清除查询条件',
                  footer: {
                    selectText: '选择',
                    navigateText: '切换',
                    closeText: '关闭'
                  }
                }
              }
            }
          }
        }
      }
    },
    locales: {
      root: {
        label: 'English',
        lang: 'en-US',
        description: 'Zero-config CLI ops agent with centralized API key custody, model failover, command guardrails, and audit logging.',
        themeConfig: {
          nav: englishNav(),
          sidebar: englishSidebar(),
          outline: { label: 'On this page', level: [2, 3] },
          footer: {
            message: 'Licensed under AGPL-3.0 (Commercial licensing available: contact@aistrun.cc)',
            copyright: 'Copyright © AIST Contributors'
          }
        }
      },
      zh: {
        label: '简体中文',
        lang: 'zh-CN',
        link: '/zh/',
        description: '面向服务器与终端控制台的零配置 On-Call 运维智能体，支持服务端统一密钥托管、模型自动容灾、命令黑名单与操作审计。',
        themeConfig: {
          nav: chineseNav(),
          sidebar: chineseSidebar(),
          outline: { label: '本页目录', level: [2, 3] },
          docFooter: {
            prev: '上一页',
            next: '下一页'
          },
          langMenuLabel: '切换语言',
          returnToTopLabel: '回到顶部',
          sidebarMenuLabel: '菜单',
          darkModeSwitchLabel: '外观',
          lightModeSwitchTitle: '切换到浅色模式',
          darkModeSwitchTitle: '切换到深色模式',
          footer: {
            message: '基于 AGPL-3.0 协议开源（提供商业授权：contact@aistrun.cc）',
            copyright: 'Copyright © AIST Contributors'
          }
        }
      }
    }
  })
)
