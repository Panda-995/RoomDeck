# Docker 部署 / Deployment

## 默认安装

安装并启动 Docker Engine / Docker Desktop 和 Compose v2，在项目根目录运行：

```sh
docker compose up -d
docker compose logs roomdeck
```

打开 `http://服务器IP:8080`，填写日志中的 First-run setup token，创建管理员。令牌也保存在 `/data/setup-token`，使用后删除。初始化之后只能由管理员创建房间；访客扫码即可加入。

**English:** Run the commands above, open port 8080, and initialize the administrator using the one-time token. Guests never need this token or an administrator account.

## 默认完整部署

Vue 静态资源、Go 服务、SQLite 驱动、图片处理、游戏、二维码、WebSocket、ZIP 与清理任务内置于 RoomDeck。新增投屏由同一 Compose 内的 LiveKit 提供，一次性初始化服务自动生成并保留密钥。默认启动包含全部已实现功能，不需要额外数据库、Redis 或图片服务。

投屏的 HTTPS、媒体 IP、端口和 TURN/TLS 说明见 [多人投屏部署](SCREEN-SHARING.md)。媒体故障不阻塞旧功能和游戏。原来的单命令不变，但网络条件需要满足，不能将 HTTP-only 转发当作媒体转发。

固定版本基础镜像，CGO 关闭，运行镜像包含 CA 证书和时区数据，以 UID/GID 10001:10001 运行。健康检查内置，不依赖 curl。

默认使用命名卷，自动保留 `/data` 权限。媒体密钥另保存在 `roomdeck-media-config` 卷。普通停止使用 `docker compose stop` 或 `down`。**不要用 `down -v` 作为停止命令，它会删除数据卷及媒体密钥。**

如改用 NAS 指定目录 `./data:/data`，需要事先授权 UID 10001 可读写。bind mount 不是默认安装前置要求。数据库使用本地可锁定文件系统，不建议放在 SMB/NFS 网络盘上。应用禁止两个进程同时打开同一数据目录。

### 将全部数据映射到宿主机目录

复制 `.env.example` 为 `.env`，设置 `ROOMDECK_DATA_PATH=./data`，再运行原来的启动命令即可。也可使用 Linux/NAS 绝对路径（例如 `/volume1/docker/roomdeck/data`）；Windows 使用正斜线，例如 `G:/RoomDeck/data`。路径对应 Docker 所在主机。

Linux/NAS 新目录准备示例（只对新建的专用目录操作）：

```sh
mkdir -p ./data
sudo chown 10001:10001 ./data
docker compose up -d
```

映射覆盖整个 `/data`：`roomdeck.db` 与运行中的 WAL/SHM、`rooms/` 内原件/预览/缩略图/导出、`uploads/` 内可恢复分片、`tmp/` 临时处理文件及内部状态。便签、链接、投票、表情和队列保存在数据库，不是独立文本文件。照片与附件使用内容 ID 命名，原文件名在数据库记录，下载与导出时恢复可读名称。

2026-09-11 功能更新将数据库迁移至 v6，不新增 Compose 服务、端口或必填环境变量。升级前停止应用并备份完整数据目录；旧版程序无法直接打开新版数据库，回滚时应恢复匹配的备份。续传合并期间需要容纳分片与成品的临时磁盘空间。

**已有命名卷不能直接切换到空目录当作迁移。** 先按下方备份步骤停服、复制完整 `/data`，将备份的内容放入目标目录、恢复权限，再设置变量重建容器。核对房间、原件和导出后再处理旧卷。不要只复制照片，数据库包含文件对应关系与访问权限。

宿主机目录仍受房间到期删除规则管理；映射不是永久备份。不要直接重命名或删除内部文件，也不要让 Web 服务器公开此目录。所有下载仍通过 RoomDeck 权限检查。

**English:** Set `ROOMDECK_DATA_PATH=./data` in `.env` to bind the entire data directory to the host. Leave it unset to retain the named volume. Prepare Linux ownership as 10001:10001. Existing installations must stop and migrate the complete data directory before switching. Mapped files still expire with their rooms; a bind mount is not a backup.

## 地址与反向代理

BASE_URL 是可选项，配置完整的 HTTP/HTTPS origin，不含路径：

```dotenv
BASE_URL=https://room.example.com
TZ=Asia/Shanghai
```

访问域名应与 BASE_URL 一致。局域网手机要能访问服务器 IP；Wi-Fi 客户端隔离、跨网络和 NAT 需要在网络层解决。Docker 不会自动提供内网穿透。

Nginx 示例，放在已配置证书的 server 中：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
    proxy_request_buffering off;
    client_max_body_size 210m;
}
```

如果代理也在容器中，使用同网络服务名替代 127.0.0.1。不能靠关闭 WebSocket 或缩小上传上限来简化代理配置。应用单文件上限为 200 MB。

HTTP 局域网支持上传、附件下载和实时共享。剪贴板不可用时显示可选中文本；全屏必须点击，不支持全屏的浏览器仍可展示。公网应配置 HTTPS，HTTP 不加密会话与内容。

## 停止、升级与备份

停止 `docker compose stop`，恢复 `docker compose start`。升级源码后 `docker compose up -d`。

备份前停止服务，复制完整 `/data`，不能只复制运行中的 SQLite 数据文件：

```sh
docker compose stop roomdeck
docker compose cp roomdeck:/data ./roomdeck-backup
docker compose start roomdeck
```

确认备份中有 roomdeck.db、rooms 及状态文件。恢复时先停止目标服务，将备份内容复制到目标卷 `/data` 根目录并恢复 10001:10001 权限。先在隔离实例演练，不直接覆盖生产数据。房间 ZIP 是内容导出，不包含完整账号/会话状态，不能代替整站备份。

新增游戏已写入同一个数据库。新版本自动将 v1 数据库迁移为 v2；升级前备份，回滚需恢复匹配版本的完整备份，不直接用旧二进制打开新数据库。媒体配置可在停服时另用 `docker compose cp roomdeck:/media-config ./roomdeck-media-backup` 备份，证书和 `.env` 也应按自己的运维策略保存；媒体备份包含密钥，不可公开。

## 排查

```sh
docker compose ps
docker compose logs --tail=100 roomdeck
```

`/healthz` 检查存活，`/readyz` 检查数据库就绪。上传前要求至少 256 MB 剩余保护空间；导出额外检查快照和 ZIP 空间。图片处理并发为 1，避免低内存主机同时解码大图。不要公开含首次初始化令牌的日志。

## 验证口径

基础镜像标签及 amd64/arm64 架构已核验。开发环境没有 Docker daemon，因此 Linux 交叉编译、原生集成测试和浏览器测试不等于容器实机通过。CI 已配置容器构建、启动和就绪检查，需在 Docker 主机实际执行。

**English summary:** The complete default Compose stack contains RoomDeck, LiveKit and automatic media-key initialization. Preserve application and media-config volumes, upload/WebSocket support, and the required media ports. Back up the entire application directory while stopped; games share that database. Do not scale replicas against one SQLite directory. Set BASE_URL for a stable external origin. Official Compose CLI parsing is verified; container runtime validation still requires a Docker daemon.

## 镜像与自动构建

默认镜像为 `ghcr.io/panda-995/roomdeck:latest`，支持 Linux amd64、arm64。可通过 `ROOMDECK_IMAGE` 固定版本或摘要。更新时先备份数据，再执行 `docker compose pull` 和 `docker compose up -d`。画板与协同主持使用数据库 v7，升级前备份完整数据目录。

从源码构建：`docker compose -f compose.yaml -f compose.build.yaml up -d --build`。初始化脚本内置于应用镜像，默认部署无需下载脚本。HTTPS 附加配置仍需仓库中的 Caddyfile。

GitHub Actions 在两种原生架构上执行 Go 检查、前端构建、中英文校验、真实容器端到端测试和重启持久化验证，通过后发布各架构并合并镜像。main 更新 latest，版本标签发布同名镜像，同时保留 commit SHA 标签。
