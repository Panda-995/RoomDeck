# RoomDeck

**给现实中的一个房间，再开一个数字房间。**

[English](README.en.md) · [部署](docs/DEPLOYMENT.md) · [开发与测试](docs/DEVELOPMENT.md) · [实现状态](docs/IMPLEMENTATION.md)

已加入表情与弹幕、断线恢复与上传续传、投屏排队、指定投票上屏，见 [功能说明与边界](docs/FEATURES-2026-09.md) 和 [界面预览](docs/features-preview.html)。

RoomDeck 是自托管的现场临时共享空间。参与者扫码或输入房间码，无需账号即可分享照片、文件、便签和链接，并参与投票。主持人选择内容上屏，在手机或电脑上控制电视/投影，活动结束后导出内容并自动清理。

[NAS 部署包 / NAS deployment bundle](https://github.com/Panda-995/RoomDeck/releases/download/nas-deploy-20260913/RoomDeck-NAS.zip) · [SHA256](https://github.com/Panda-995/RoomDeck/releases/download/nas-deploy-20260913/RoomDeck-NAS-SHA256SUMS.txt)

## Docker 启动

NAS 新安装可使用 [NAS 专用配置](compose.nas.yaml)：只需填写 NAS 局域网 IP，无需 `.env` 或外部初始化脚本。服务说明、数据映射与已有部署切换方法见 [NAS 部署](docs/NAS.md)。

下载本仓库的 `compose.yaml` 即可部署。镜像 `ghcr.io/panda-995/roomdeck:latest` 支持 Linux amd64 / arm64。

```sh
docker compose pull
docker compose up -d
docker compose logs roomdeck
```

打开 `http://服务器IP:8080`，用日志中的 **First-run setup token** 完成首次初始化，管理员密码在网页设置。初始化令牌验证后失效，无需自己配置数据库或生成密钥。

默认使用命名数据卷，也可通过 `.env` 的 `ROOMDECK_DATA_PATH=./data` 映射全部业务数据。Compose 同时启动非 root 的 RoomDeck 应用和 LiveKit 媒体服务，并自动初始化媒体密钥。部署方式不会关闭已实现功能，前端不依赖 CDN、外部字体或图片 API。

多人投屏需要正确的媒体 IP、端口和 HTTPS；可复用现有反代，或使用附带的自动 HTTPS 配置，详见 [投屏部署](docs/SCREEN-SHARING.md)。普通局域网 HTTP 可以共享文件，但不能保证允许浏览器捕获屏幕。

使用域名时将 `.env.example` 复制为 `.env`，设置 `BASE_URL` 为参与者可访问的完整地址，再执行启动命令。局域网可直接使用服务器 IP；不要使用 localhost 邀请其他设备。

## 已实现

- 简体中文 / English，自动识别浏览器语言并保存切换偏好。
- 初始化、登录、房间预设与功能开关、保留期限、人数/容量限制。
- 六位房间码、二维码、邀请轮换、昵称会话、成员移出/暂停上传。
- 照片上传、方向校正、去元数据预览、文件附件下载。
- 便签、链接、匿名单选/多选投票、结果隐藏与修改投票。
- WebSocket 实时更新与断线状态恢复。
- 内容精选/隐藏/删除，独立只读大屏、轮播、暂停与黑屏。
- ZIP 导出、校验清单、到期删除、重启清理和单实例数据锁。
- 宿主机数据映射，管理员可下载原文件名与磁盘路径的对应清单。
- 每位成员可轮流投屏，多位观众与大屏同时观看，主持人可终止共享。
- [谁是卧底与骰子猜点数](docs/GAMES.md)：私人信息、回合结算、积分与重连恢复。

## 边界

照片支持 JPEG/PNG/WebP；HEIC 和视频可作为普通文件共享，暂未实现 HEIC 预览和视频转码。默认原件下载关闭，原件可能保留拍摄位置信息。匿名投票按浏览器会话去重，不验证真实身份。

当前为单实例开发版，最多 100 个房间、每房 2,000 条内容。默认每房 2 GB、50 人，设置中可调整容量与人数。更多阶段性实现差异见 [实现状态](docs/IMPLEMENTATION.md)，不能把原始设计文档的全部未来功能视为已完成。

## 本地开发

需要 Go 1.27.1、Node.js 22.22.2 或兼容版本：

```sh
cd web
npm ci
npm run build
cd ..
go run ./cmd/roomdeck
```

默认访问 `http://localhost:8080`，数据存储在 `./data`。验证命令和浏览器测试见开发文档。
原生开发需额外启动 LiveKit 并配置媒体环境变量才能投屏，其他功能可独立运行。

## 原始设计交付

当前正式界面采用暖白与深绿色的轻简风格。新版实机截图及改版前后对照可打开 [界面预览](docs/ui-preview.html)，设计规范见 [DESIGN.md](DESIGN.md)，检查与验证记录见 [UI / UX 改版记录](docs/UI-UX-AUDIT.md)。

界面包含导航、弹窗、卡片与状态反馈动效，可观看 [桌面与手机录屏](docs/ui-motion.html)。

正式源码位于 `cmd/`、`internal/`、`web/`。`prototype/` 和 `previews/` 是此前的静态设计预览，含演示数据，**不是正式业务程序**。设计文档在 `docs/RoomDeck-开发设计文档.md`。

## 示例照片来源

通过 Unsplash images 服务获取，用于设计示例。正式上线应替换为自己的活动照片，并核查使用条件。

- https://images.unsplash.com/photo-1528605248644-14dd04022da1
- https://images.unsplash.com/photo-1511795409834-ef04bbd61622
- https://images.unsplash.com/photo-1414235077428-338989a2e8c0
- https://images.unsplash.com/photo-1555939594-58d7cb561ad1

二维码为 https://roomdeck.example/join/7K3PM9 的真实编码，仅示范版式，不是有效活动入口。


新增：共同画板、限时你画我猜与按职责授权的协同主持。入口及部署说明见 [画板与协同主持](docs/BOARD-AND-COHOSTS.md)。
