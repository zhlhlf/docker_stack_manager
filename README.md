# Docker Stack Manager

轻量级 Docker Swarm Stack 服务管理面板。

## 功能
- Stack 与端口白名单管理（SQLite）
- 实时检测服务归属与端口违规
- 一键清理 / 定时自动清理
- x-ui 风格 Web UI

## 运行

```bash
go mod tidy
go run .
```

默认监听 `:8080`，打开 http://localhost:8080

### 参数 / 环境变量
- `-addr` / `LISTEN_ADDR`  默认 `:8080`
- `-db` / `DB_PATH`        默认 `data.db`
- `-static` / `STATIC_DIR` 默认 `static`

### Docker 权限
需要可访问 Docker Engine（`DOCKER_HOST` 或本地 pipe/socket），并具备：
- ServiceList
- ServiceInspect
- ServiceRemove

建议以挂载 Docker socket 的方式部署到管理节点。

## 违规规则
1. 无法归属到已配置 Stack（标签 `com.docker.stack.namespace` 优先，其次服务名前缀）
2. 已归属 Stack，但发布端口不在白名单（白名单为空 = 不允许任何发布端口）
3. 无发布端口的服务不判定端口违规
