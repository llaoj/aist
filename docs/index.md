---
layout: home

hero:
  name: AIST
  text: On-Call AI Shell Assistant
  tagline: Natural-language server diagnostics, human-confirmed execution, multi-host SSH, and centralized team governance — self-hosted with zero client config.
  image:
    light: /logo.svg
    dark: /logo-bw.svg
    alt: AIST
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started/overview
    - theme: alt
      text: 5-Min Self-Hosting
      link: /getting-started/self-hosting
    - theme: alt
      text: Try Online
      link: https://aistrun.cc

features:
  - title: Zero-Config Thin Client
    details: One bootstrap command installs aist and connects to your server. API keys, upstream URLs, and model routing never touch developer or production machines.
  - title: Confirm Before Every Action
    details: Commands are classified as Query or Modify, checked against AST-parsed blacklists and full-screen TUI guards, and executed in a real PTY only after your approval.
  - title: Multi-Host SSH & Safe File Edit
    details: Troubleshoot local machines and remote SSH targets in one loop, with sudo PTY passthrough and SHA-256 CAS-verified atomic file edits backed up locally.
  - title: Resilient Model Priority & Failover
    details: Chain free-tier, trial, and paid OpenAI-compatible providers by priority. Automatic error classification cools down exhausted or rate-limited models and recovers them on schedule.
  - title: Team Governance & Audit Trail
    details: Manage invite codes, new-user templates, weekly UTC token quotas, daily usage metering, and shell/file_edit audit logs directly in the built-in admin console.
  - title: Air-Gapped Ready
    details: The ~42 MB server image bundles matching multi-platform client archives and static web assets. Deploy inside private networks alongside vLLM or Ollama with zero internet dependency.
---
