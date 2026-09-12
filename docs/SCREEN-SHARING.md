# 多人投屏部署与使用 / Screen sharing

当前实现：每位房间成员都可以发起屏幕共享，同一时间一位分享者，多个成员和独立大屏同时观看。自由轮流，主持人可以结束任何人的投屏。没有多人画面拼接、录制或手机原生投屏客户端。

## 一条命令启动

默认 `compose.yaml` 已包含 RoomDeck、LiveKit 和一次性密钥初始化服务：

```sh
docker compose up -d
```

初始化自动生成随机媒体密钥，保存于 `roomdeck-media-config` 卷。以后启动复用密钥，不需要手抄到前后端。媒体服务未就绪时页面会说明，照片、文件和游戏继续工作。应用不等待媒体健康状态才启动。

旧版已有照片数据卷仍使用原名，未设置 `ROOMDECK_DATA_PATH` 时不会切换存储。设置 `ROOMDECK_DATA_PATH=./data` 可将数据库、照片、附件、导出映射到宿主机；迁移旧数据的方法见 [部署文档](DEPLOYMENT.md)。

## 网络准备

同机访问 `http://localhost:8080` 可开发测试；其他设备发起投屏要通过受信任 HTTPS。`http://192.168.x.x:8080` 能访问房间，不代表允许屏幕捕获。

设置 `.env` 的 `ROOMDECK_MEDIA_IP` 为观众能访问的服务器 IP。NAS 局域网示例：`ROOMDECK_MEDIA_IP=192.168.1.100`；公网实例填写公网 IP。不填会由 LiveKit 尝试发现公网地址，不适用于所有 NAS/NAT 环境。修改媒体配置后先结束投屏，再执行 `docker compose up -d --force-recreate` 使初始化与媒体服务读取新配置。

|端口|用途|要求|
|---|---|---|
|8080 TCP|RoomDeck 网页、API、经过权限校验的媒体信令|已有反代转发到这里|
|7881 TCP|WebRTC TCP 回退|放行/转发到 Docker 主机同端口|
|7882 UDP|WebRTC 媒体|放行/转发到 Docker 主机同端口|
|3478 UDP|内置 TURN/UDP 中继|放行/转发到 Docker 主机同端口|
|80/443 TCP|可选 HTTPS 入口|启用 HTTPS 配套文件时使用|

**不要将 LiveKit 的 7880 管理/信令端口直接映射到公网，也不要另设反向代理绕开 RoomDeck `/media/`。** RoomDeck 在此校验房间、成员、会话和当前投屏，包含媒体服务刷新后的连接凭证。独立大屏只能订阅；分享者只获屏幕和屏幕音频的发布权限。内部服务网络、Docker 配置和管理员密钥由部署管理员管理。

反向代理必须支持 `/media/` 的 WebSocket 升级，与现有房间 WebSocket 一并转发；不要记录含 `access_token` 的查询字符串。媒体数据走上表的媒体端口，不会被 HTTP-only 内网穿透自动转发。

## 自动 HTTPS（可选）

已有域名和反代可继续复用。没有反代时，在 `.env` 中设置：

```dotenv
ROOMDECK_DOMAIN=room.example.com
BASE_URL=https://room.example.com
ROOMDECK_MEDIA_IP=你的服务器公网IP
```

域名指向服务器，放行 80/443，然后启动：

```sh
docker compose -f compose.yaml -f compose.https.yaml up -d
```

Caddy 自动申请并续期网页证书；以后停止、升级应继续使用同一组 `-f` 参数。局域网专用域名可复用你已有的受信任证书入口，不要把自签名证书警告当成已经验证的 HTTPS。

## 限制网络中的 TURN/TLS

默认同时提供直连 UDP、TCP 和 TURN/UDP。有些企业/访客网络只允许 TLS 443，此时增加 TURN/TLS。

仓库提供 `compose.turn-tls.yaml`。准备一个与网页不同的服务器可绑定 IP，以及指向它的 TURN 域名。将该域名的证书放到 `ROOMDECK_TURN_CERTS` 目录中的 `fullchain.pem`、`privkey.pem`。示例中的地址仅用于说明，必须替换为实际地址：

```dotenv
ROOMDECK_TURN_DOMAIN=turn.example.com
ROOMDECK_TURN_CERTS=./deploy/turn-certs
ROOMDECK_TURN_BIND_IP=203.0.113.11
ROOMDECK_WEB_BIND_IP=203.0.113.10
```

```sh
docker compose -f compose.yaml -f compose.https.yaml -f compose.turn-tls.yaml up -d
```

网页 HTTPS 和 TURN/TLS 都使用 443，因此本配套方案要求分别绑定 IP。不能让网页绑定 `0.0.0.0:443` 再让 TURN 占同一端口。云 NAT 场景应填写对应本机可绑定地址并配置公网转发。只有一个 IP 时，可使用已配置的四层分流入口或另外部署 TURN 主机，需要按实际网络调整；不把这个进阶拓扑伪装成默认自动完成。

TURN 证书需由你的证书工具维护，替换后重启 LiveKit 加载。文件夹中只放证书，不将媒体密钥放在公开 Web 目录。实机还需验证强制中继路径，配置存在不代表所有网络已经连通。

## 页面操作

1. 进入房间的“投屏”分类，点击“分享我的屏幕”。浏览器每次都需要你选择来源并授权。
2. 其他成员点击“观看投屏”。配对的大屏自动播放；若声音被浏览器阻止，点击“开启播放与声音”。
3. 分享者或主持人点击“结束所有人的投屏”。停止后大屏恢复原来的照片/欢迎模式。
4. 大屏黑屏会停止该大屏的音视频接收；房间成员仍可继续看。需要全部停止时使用投屏里的结束按钮。

分享者离开页面、停止系统捕获、被移除或房间关闭后，服务会收回投屏。正常服务下后台每 3 秒检查权限；心跳租约 45 秒。网络故障可能延迟清理，因此客户端也在连续请求失败时关闭媒体。应用重启终止旧投屏，不自动重新打开用户屏幕。

手机网页通常只能观看，不能承诺 iPhone/Android 都能发起整机录屏。音频是否能捕获取决于操作系统、浏览器和共享来源。这里的大屏是浏览器页面，不是 AirPlay/Miracast 协议接收器。

## 验证与边界

本地已使用 LiveKit 1.13.6 与 Chromium 完成真实 SFU 连接和多端收流测试；捕获来源采用浏览器自动化测试画面。未实测公网 NAT、强制 TURN/TLS、真实手机/电视或 Docker 引擎运行。Compose 文件已用官方 Compose CLI 验证解析，不能等同于容器实机验收。

**English:** The default Compose stack includes RoomDeck, LiveKit and automatic persistent key initialization. Everyone may publish in turn; multiple guests and the paired display may watch. Use trusted HTTPS to capture from devices other than localhost, set the reachable media IP, and open 7881/TCP, 7882/UDP and 3478/UDP. Keep LiveKit signaling port 7880 private: all signaling must pass through RoomDeck `/media/`. Optional Caddy HTTPS and dedicated-IP TURN/TLS overlays are provided. Screen capture is not recorded; mobile publishing and system audio are browser-dependent. Real-device, cross-network and Docker-runtime checks remain required.

参考：[LiveKit 部署](https://docs.livekit.io/transport/self-hosting/deployment/)、[屏幕捕获 API](https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getDisplayMedia)。
