# Docker Stack Manager

轻量级 Docker Swarm Stack 服务管理面板。

## 功能
- Stack 与端口白名单管理
- **真实 Docker Swarm Service** 归属/端口违规检测
- 一键清理 / 定时自动清理
- 前端 `go:embed` 打进单二进制
- x-ui 风格 Web UI

## 运行（需要可访问 Docker Engine）

```bash
# 本机（Linux / 已装 Docker）
go run .
# 或
./docker_stack_manager_linux_amd64 -addr :8080
```

浏览器：http://localhost:8080

### 容器运行（推荐）

```bash
docker run -d --name stack-manager \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v stack-manager-data:/data \
  ghcr.io/zhlhlf/docker_stack_manager:latest
```

> 必须挂载 Docker socket，程序通过 Engine API 读取/删除 Swarm Service。

### 参数 / 环境变量
- `-addr` / `LISTEN_ADDR`  默认 `:8080`
- `-db` / `DB_PATH`        默认 `data.json`

### 权限
Docker 需要：
- `ServiceList`
- `ServiceInspect`（列表已含）
- `ServiceRemove`

建议跑在 Swarm manager 节点。

## 违规规则
1. 无法归属到已配置 Stack  
   - 优先标签 `com.docker.stack.namespace`  
   - 其次服务名前缀 `<stack>_<service>` 且 Stack 已配置
2. 已归属 Stack，但**发布端口**不在白名单  
   - 白名单为空 = 不允许任何发布端口
3. 无发布端口的服务不判定端口违规

## CI
- push `main` 触发 Release
- tag = `v` + commit 前 8 位
- 仅构建 Linux amd64