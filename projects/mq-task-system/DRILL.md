# 演练：制造积压 → 恢复时间 → 出报告

本演练目标：让你能真实填充简历里的 `{msg_per_sec}/{backlog}/{minutes}`，并形成可截图的证据链（Grafana曲线 + RabbitMQ面板 + 报告）。

## 你会得到的产出物

- 1 张 Grafana 截图：
  - `Backlog (Queue Depth)`：`tasks.q` 从上升到回落
  - `Consume QPS`：并发提升后吞吐上升
- 1 张 RabbitMQ 管理台截图：队列深度变化
- 1 份报告：`reports/drill_report.md`

## 前置

在 `projects/mq-task-system` 目录：

1. 启动依赖

```bash
docker compose up -d
```

2. 启动 consumer（低并发）

Windows PowerShell 示例：

```powershell
$env:WORKERS="2"
$env:PREFETCH="2"
go run ./cmd/consumer
```

3. 发送大量任务

```bash
go run ./cmd/producer -n 5000 -concurrency 20
```

## 观察积压

- 打开 RabbitMQ 管理台： http://localhost:15672
- 查看 `tasks.q` 的 `Messages` 是否持续上升
- 打开 Grafana： http://localhost:3001
  - Dashboard: `mq-task-system`
  - 看 `Backlog (Queue Depth)` 面板，`tasks.q` 应该上升

记录此时的峰值积压：`{backlog_peak}`

## 恢复：提升 consumer 并发

停止 consumer（Ctrl+C），然后以更高并发启动：

```powershell
$env:WORKERS="20"
$env:PREFETCH="20"
go run ./cmd/consumer
```

观察：

- `Consume QPS` 上升
- `Backlog (Queue Depth)` 开始回落

记录恢复时间：

- 从积压峰值开始到 `tasks.q` 回落到接近 0 的时间差：`{minutes}`

## 报告模板

将你的数据填入：`reports/drill_report.md`
