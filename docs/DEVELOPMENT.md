# 开发与测试 / Development

## 目录

`cmd/roomdeck` 为服务入口；`internal/server` 按 auth、rooms、content、assets、realtime、jobs、recovery 分文件，schema.sql 为数据库 v1；`web/src/views` 页面，components 公共组件，locales 中英词典，roomSync.ts 实时同步。原始 prototype 仅作设计参考。

## 本地构建

```sh
cd web
npm ci
npm run build
cd ..
go run ./cmd/roomdeck
```

默认 DATA_DIR=data、WEB_DIR=web/dist、LISTEN_ADDR=:8080。热更新可在 web 中运行 `npm run dev`，代理 API/WS 到 8080；开发时不配置跨域 BASE_URL。

## 检查

```sh
go test ./... -count=1
go vet ./...
cd web
npm run build
npm run test:i18n
npm audit --omit=dev --registry=https://registry.npmjs.org
```

Go 使用 gofmt；前端 `npm run format`。新增文案必须补齐两种语言。`go test -race ./...` 需要 C 工具链，在 Linux CI 执行；没有执行就不能视为 race 通过。

## 浏览器测试

使用专门数据目录，不能指向真实用户数据。测试创建 e2e-host 和测试活动。

终端一（POSIX）：

```sh
go build -o dist/roomdeck ./cmd/roomdeck
DATA_DIR=.local/e2e-data LISTEN_ADDR=127.0.0.1:8093 ./dist/roomdeck
```

终端二：

```sh
cd web
npx playwright install chromium
npm run test:e2e
```

PowerShell 使用 `$env:DATA_DIR` 和 `$env:LISTEN_ADDR`。可通过 ROOMDECK_TEST_URL、ROOMDECK_SETUP_TOKEN_FILE、PLAYWRIGHT_CHROMIUM_EXECUTABLE 覆盖测试地址/令牌文件/浏览器路径。

流程覆盖英文创建、中文加入、照片上传/精选/大屏、便签、投票、黑屏、ZIP、结束、中英切换和 320/390/768px 布局。报告在 web/playwright-report，截图在 web/test-results。

新增 `activities.spec.ts` 覆盖双语游戏、私密视图和恢复；默认跳过需要真实 SFU 的媒体用例。执行媒体用例时，先启动 LiveKit 1.13.6，使用 `web/tests/fixtures/livekit.yaml`（仅隔离测试），再给 Go 进程配置：

```dotenv
LIVEKIT_INTERNAL_URL=http://127.0.0.1:7880
LIVEKIT_API_KEY=roomdeck-test
LIVEKIT_API_SECRET=test-only-secret-for-isolated-local-tests-2026
```

测试进程设置 `ROOMDECK_MEDIA_TEST=1` 即可执行完整媒体用例。测试浏览器自动授权并生成捕获画面，通过真实 SFU 传输；不会调用外部商业媒体服务。它验证多端收流、客人发起与主持人停止，不等同于实际手机、电视或跨公网测试。CI 同样下载并验证 LiveKit 发布包的 SHA256 后运行。

Go 新增覆盖 v1 数据迁移、路径映射、游戏私密视图、并发猜测、重复结算防护、平票、成员移除、游戏重启恢复、媒体授权和旧凭证失效。游戏状态通过专用接口每 1.5 秒刷新，媒体状态每 2 秒刷新；打开对应分类后才启用，投屏 SDK 按需加载。

`network.spec.ts` 使用浏览器虚拟时钟验证 15 秒 JSON 请求超时，并模拟反代 HTML 错误。超时仅结束客户端等待，服务端可能已完成写入，因此不自动重发写操作。文件流式上传独立处理，不受该超时限制。`media_failure_test.go` 覆盖媒体创建失败、清理失败与取消请求；缺失资源只有在删除操作中才视为成功。

容器检查执行 `docker compose build`、`docker compose up -d --wait`，再手工验证初始化、上传、大屏和重启持久化。使用独立 Compose 项目名隔离测试卷。

后续优先：图片持久任务与续传、分页与增量事件、大屏渲染 ACK、更新导出、HEIC 真机验证、Moderator/审核。增加能力时保持默认 Docker 安装完整可用。
