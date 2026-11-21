---
layout: home

hero:
  name: AIST
  text: 常驻终端的 On-Call 运维智能体
  tagline: 自然语言排障、人工确认后执行、多主机 SSH 协同与服务端统一治理——客户端零配置，支持纯内网私有化部署。
  image:
    light: /logo.svg
    dark: /logo-bw.svg
    alt: AIST
  actions:
    - theme: brand
      text: 快速了解
      link: /zh/getting-started/overview
    - theme: alt
      text: 5 分钟私有部署
      link: /zh/getting-started/self-hosting
    - theme: alt
      text: 在线体验
      link: https://aistrun.cc

features:
  - title: 零配置轻量客户端
    details: 一行脚本完成安装、校验与登录。真实模型 API Key、上游地址与路由策略全部收在服务端，绝不下发到终端或生产机器。
  - title: 先确认、后执行的安全闭环
    details: 自动区分「查询命令」与「修改命令」，内置基于 Shell AST 的命令黑名单与全屏交互程序拦截，所有变更均展示原因或彩色 Diff 并经人工确认。
  - title: 多主机 SSH 与原子文件编辑
    details: 同一对话串联本机与多台远端 SSH 主机，原生支持 PTY 交互（sudo / 密码 / MFA），文件修改带 SHA-256 并发校验与自动本地历史备份。
  - title: 模型优先级路由与自动容灾
    details: 按优先级串联免费额度、赠金与主力付费模型；精准识别余额耗尽、限流与鉴权错误，自动冷却退避并在额度恢复后重新启用。
  - title: 团队管控与操作审计
    details: 开箱即用管理邀请码、新用户模板、每周 Token 限额、按日用量统计，以及自动采集写入后台的 shell_execute / file_edit 审计日志。
  - title: 纯内网离线可用
    details: 约 42 MB 的单容器服务端镜像已内置全平台同版本客户端安装包与静态站点，配合 vLLM / Ollama 即可在完全断网环境闭环运行。
---
