# 更新日志 / Changelog

## 2026-09-13 — 大屏模式与上屏操作名称

- 大屏“照片”改为“照片轮播”，“精选”改为“便签与链接”，明确照片播放和文字内容展示的区别。
- 内容卡片统一使用“加入上屏”“移出上屏”“已加入上屏”。加入上屏表示内容具备展示资格，不表示正在大屏显示，也不会自动切换展示模式。
- 修正“便签与链接”空状态误提示等待照片的问题。
- 同步调整照片等待提示、自动上屏说明、分享提示及英文翻译。照片上传入口保持“照片”。
- 不修改内容筛选、播放规则、权限、数据结构或已选内容；无需数据迁移。
- 延续 GitHub Actions 原生 amd64/arm64 测试与发布流程，以及发布后的匿名双架构镜像检查。
- 标准 Compose 和 NAS Compose 均使用公开镜像 `ghcr.io/panda-995/roomdeck:latest`，支持自动选择主机架构。

English: Big-screen modes are now **Photo slideshow** and **Notes & links**. Content actions use **Add to display**, **Remove from display** and **Added to display**. Adding content makes it eligible for display; it does not switch the active mode. Upload categories, playback behavior, permissions and stored data are unchanged. No database migration is required.

### 更新 / Update

在现有 Compose 项目目录执行，保留原项目名称、数据卷及环境配置：

```sh
docker compose pull
docker compose up -d
```

NAS 部署说明：[docs/NAS.md](docs/NAS.md)。更新前备份业务数据与媒体配置；更新容器会中断正在进行的投屏。
