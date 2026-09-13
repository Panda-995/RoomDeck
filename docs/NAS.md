# NAS 部署：一个文件、一个必填项

适用于提供 Docker Compose 项目管理功能的 NAS，镜像支持 Linux amd64 / arm64。不同品牌的菜单名称会有所不同，本教程按通用的“项目 / Compose”方式操作。

## 1. 下载和准备

下载 [NAS 部署包](https://github.com/Panda-995/RoomDeck/releases/download/nas-deploy-20260913/RoomDeck-NAS.zip)，解压后可直接使用包内的 `compose.yaml`。也可以单独下载仓库的 [compose.nas.yaml](https://github.com/Panda-995/RoomDeck/blob/main/compose.nas.yaml)。包内包含完整教程和可选 HTTPS/TURN 配置；常规安装只需要 Compose 文件。

先准备以下内容：

1. 确认 NAS 容器管理器已启用，支持 Compose v2 和服务完成条件 `service_completed_successfully`。
2. 查询 NAS 当前局域网 IP；建议在路由器为 NAS 保留固定地址，避免重启后投屏地址变化。
3. 在 NAS 文件管理器建立专用项目目录，例如已有 Docker 共享目录下的 `roomdeck`，将部署包解压到这里。
4. 确认 8080/TCP、7881/TCP、7882/UDP、3478/UDP 没有被其他应用占用，并允许参与设备访问。

## 2. 修改 IP 并导入项目

使用 `compose.nas.yaml`，将其中 `ROOMDECK_MEDIA_IP` 的示例地址改为 NAS 实际局域网 IP。在 NAS 的 Compose/项目管理界面导入；新安装可将文件命名为 `compose.yaml`。无需 `.env`、外部初始化脚本或手工填写密钥。

例如 NAS 地址是 `192.168.31.20`，将对应一行改成：

```yaml
      ROOMDECK_MEDIA_IP: "192.168.31.20"
```

在容器管理器选择创建 Compose 项目，项目名填 `roomdeck`，选择刚才的项目目录及 Compose 文件。若界面只允许粘贴 YAML，可以复制文件完整内容。创建时允许拉取镜像并启动；后续更新也使用同一个项目名称和目录。

不要将三项服务拆成三个独立的容器向导：它们需要共享项目网络、数据卷和启动顺序。若 NAS 界面不支持 Compose，可通过 SSH 在项目目录运行下方命令；不需要逐个配置容器。

这份配置面向局域网用户。若参与者从公网连接，需要填写他们能够访问的媒体地址，并配置对应端口转发；单独把网页反向代理出去不足以让投屏连通。

## 原配置各部分的作用

|部分|作用|需要手动管理吗|
|---|---|---|
|`roomdeck`|网页、文件、照片、投票、大屏、游戏、画板、协同主持|正常部署/更新即可|
|`livekit`|投屏音视频转发，支持多位观众|随项目启动，不用单独登录配置|
|`media-init`|首次随机生成媒体密钥，并生成两项服务共用的配置；后续复用密钥|自动运行后退出，显示 Exited (0) 属于正常状态|
|`depends_on`|保证密钥和配置生成完毕再启动服务|保留即可|
|`entrypoint`|自动读取媒体密钥后启动应用|保留即可|
|`roomdeck-data`|数据库、照片、文件、预览、导出和上传分片|需要备份|
|`roomdeck-media-config`|媒体密钥和配置|与业务数据一起备份|
|`restart` / 健康检查|NAS 重启后自动恢复常驻服务、报告应用健康状态|保留即可|
|`user` / `security_opt`|初始化数据卷权限；应用本身仍以非 root 身份运行|保留即可|

NAS 版沿用已经发布的镜像，保留三个服务，其中只有两个常驻。简化的是操作：移除默认无需使用的证书目录、镜像变量和公网选项。照片、文件、游戏、画板、恢复和投屏相关功能没有关闭。

## 启动

新安装将文件命名为 `compose.yaml` 后执行：

```sh
docker compose pull
docker compose up -d
docker compose logs roomdeck
```

打开 `http://NAS-IP:8080`，使用日志中的首次初始化令牌设置管理员。也可以直接在 NAS 的容器日志页面查看。

例如 NAS 为 `192.168.31.20`，访问 `http://192.168.31.20:8080`。在容器列表找到 `roomdeck` 服务，打开日志，复制 `First-run setup token` 后面的令牌。在网页中填写令牌、管理员用户名和至少 12 位的密码。令牌使用一次后失效，参与活动的访客不需要它。

正常状态是 `roomdeck` 和 `livekit` 运行中，`media-init` 显示已退出且退出码为 0。随后创建一个房间，让另一台设备扫码加入，试传一张照片、发一条便签、创建一个投票，再打开大屏查看。完成后重启 `roomdeck`，确认房间和照片仍可访问。

投屏发起端需要可信 HTTPS。建议复用 NAS 已有的域名、证书和反向代理，将整个站点转发到 NAS 的 8080 端口，并支持 WebSocket。没有现成入口时可使用项目原有 `compose.https.yaml` 与 `deploy/Caddyfile`。仅观看屏幕与发起屏幕捕获的浏览器要求不同；手机通常可观看，不能承诺能发起整机投屏。

测试投屏时，用电脑通过 HTTPS 打开房间，选择“投屏 → 分享我的屏幕”，授权一个窗口或标签页。再让另一台设备在同一房间点击“观看投屏”。先确认两台设备处于可以互访的网络，避免将访客 Wi-Fi 的设备隔离误认为应用故障。

## 端口

- `8080/TCP`：网页和房间连接，可将宿主机端改为其他未占用端口，例如 `8090:8080`。
- `7882/UDP`：投屏媒体数据。
- `7881/TCP`：媒体 TCP 回退。
- `3478/UDP`：内置 TURN 中继。

后三项保持原端口，并在 NAS 防火墙放行。浏览器 HTTPS 是网络要求，不会因 Compose 行数减少而消失。LiveKit 的内部 7880 端口继续不对外开放。

## 数据目录

默认命名卷由 NAS 的 Docker 管理，不需要手工处理目录权限，容器更新也不会删除数据。

希望从文件管理器访问时，将 `roomdeck` 下的 `roomdeck-data:/data` 改为 NAS 专用目录，例如 `/实际存储卷/docker/roomdeck/data:/data`。请使用设备上的真实绝对路径，并让 UID/GID `10001:10001` 可以读写。不同 NAS 的存储卷名称不同，不能直接照抄示例路径。

只对新建的专用目录设置权限；不要递归修改整个共享盘的权限。旧命名卷迁移到目录时，应先停服、备份并复制完整数据，包括数据库及媒体文件，不能只改映射后直接启动。详细迁移和备份步骤见 `DEPLOYMENT.md`。

## 已安装用户切换

保留原 Compose 项目名称、原业务数据映射和原媒体配置卷。Docker 命名卷带有项目名前缀；换项目名或换目录后新建项目可能创建空卷，看起来像数据丢失。已设置自定义目录、域名、时区或 TURN/TLS 的部署需要保留这些设置，不要直接覆盖成 NAS 默认值。

首次更换配置或修改 IP 时，先结束正在进行的投屏，再在同一项目中重新创建服务：`docker compose up -d --force-recreate`。普通更新使用 `docker compose pull` 和 `docker compose up -d`。正常停止不要使用 `down -v`，它会删除数据卷。

## 可选高级网络

NAS 版默认仍提供 UDP、TCP 和 TURN/UDP。只允许 TLS 443 的网络可叠加 `deploy/compose.nas.turn-tls.yaml`，配置 TURN 域名、证书目录和独立绑定 IP。它补回可选证书挂载，完整网络要求与证书维护方法见 `SCREEN-SHARING.md`。当前应用没有删减 TURN/TLS 能力，只是将它从默认填写项移到可选文件。

## 常见问题

|现象|先检查什么|
|---|---|
|网页打不开|容器是否运行、8080 是否冲突、NAS 防火墙是否放行；在其他设备上使用 NAS IP，不要用 localhost|
|`media-init` 显示已停止|退出码 0 是正常完成；非 0 则查看该服务日志，检查 IP 格式或可选证书设置|
|目录出现 Permission denied|自定义映射目录是否允许 UID/GID 10001 读写；不要使用放开整个共享盘权限的方式处理|
|网页能用，但无法发起投屏|是否通过浏览器信任的 HTTPS 访问、浏览器是否支持捕获、是否已授权；HTTP 局域网地址通常不满足捕获条件|
|可以发起，但其他人看不到画面|媒体 IP 是否填对，7881/TCP、7882/UDP、3478/UDP 是否可达，以及 Wi-Fi 隔离、NAT、反向代理 WebSocket 设置|
|更新后像新安装一样|项目名称或数据映射是否变了；先停止新项目并核对旧数据卷，不要继续初始化或删除旧卷|
|NAS 提示 Compose 字段不支持|确认使用的是 Compose 项目管理，并支持 v2；不要直接删掉 depends_on 条件来绕过初始化顺序|

升级前先停止应用，备份完整业务数据与媒体配置。升级后测试登录、照片下载、投票和投屏。备份恢复与命名卷迁移的具体步骤见 [部署文档](DEPLOYMENT.md)。

## 验证范围

NAS 版使用同一套已发布镜像与启动流程，保留数据卷、权限和媒体端口。配置解析以及与原配置的核心参数等价检查通过；不是新增镜像，也未在实体 NAS 上重新执行整套测试。此前公开镜像的 amd64/arm64 原生容器回归均通过，记录：https://github.com/Panda-995/RoomDeck/actions/runs/34693020045 。

## English

For a fresh LAN deployment, import `compose.nas.yaml` into your NAS Compose manager and replace the example `ROOMDECK_MEDIA_IP` with your NAS address. You may rename it to `compose.yaml`. No `.env`, external initialization script or manually entered media keys are required. The two persistent services and the one-shot initializer are unchanged.

Named volumes avoid manual directory permissions. For an existing installation, keep its project name, data mounts, media configuration volume and custom settings. Back up and migrate the complete data directory before changing mounts. Use trusted HTTPS and reachable media ports for screen capture; an HTTP reverse proxy alone cannot carry WebRTC media. TURN/TLS remains available through the optional NAS overlay. Configuration equivalence was checked; physical NAS and network-specific testing remains necessary.
