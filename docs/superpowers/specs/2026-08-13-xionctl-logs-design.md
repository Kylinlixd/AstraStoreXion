# xionctl 节点日志设计

## 目标

让服务器管理员通过 `xionctl` 查看 Xion systemd 节点日志，不必手写 `journalctl`。

## 命令

```bash
xionctl logs
xionctl logs --lines 100
xionctl logs --since 1h
xionctl logs --follow
```

命令固定读取 `astrastore-xion.service`，默认输出最近 100 行；`--since` 使用 systemd 支持的时间表达式，`--follow` 持续跟踪新日志。输出直接转发到终端，不解析、不改写日志内容。

## 安全边界

- 使用 `exec.CommandContext` 参数数组调用 `journalctl`，不经过 shell，避免命令注入。
- 不允许 CLI 用户指定任意 systemd unit，避免工具变成通用服务日志读取器。
- 日志查看只读，不重启服务、不修改数据目录。
- 保留 journalctl 的退出状态和权限错误，方便判断是无日志还是当前用户没有读取权限。

## 验证

使用可注入的 journal runner 测试参数、默认值、`--follow` 和错误传播；继续运行全仓库 Go 测试与 `go vet`，并在服务器执行 `xionctl logs --lines 5`。
