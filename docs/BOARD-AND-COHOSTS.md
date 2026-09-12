# 共同画板、你画我猜与协同主持

2026-09-12。以下描述对应当前实现，中英文界面共用同一套功能。

## 入口与使用

进入房间的「游戏」页，选择「画板 / 你画我猜」。默认进入共同画板，房间内未被禁言的成员都可以画。支持六种彩色笔、白色覆盖笔、三种笔粗、撤销自己最后一段笔画以及保存 PNG 图片。绘画使用鼠标或触摸；浏览器视口已测试，真实手机和电视仍需实机验收。

主持人或拥有「游戏与画板主持」权限的成员可以清空画板、创建猜画游戏。清空、切换到新游戏和结束游戏都有二次确认。长笔画分段提交，因此撤销针对最后一个已保存片段。未提交成功的笔画在当前页面保留，可点击重试；刷新或离开画板前，应等待「已同步」，尚未确认的笔画不保证恢复。

主持人可在「现场大屏」选择「画板」。大屏只观看，不参与绘画和猜词。与原先照片、投票和投屏一样，画板由主持人明确选择上屏。

## 你画我猜规则

- 2–12 人，按加入顺序轮流作画，每人一轮，每轮 90 秒。
- 创建游戏时选择中文或英文词库；界面语言可以独立切换。
- 只有当前画者可以绘画和查看自己的词语；词语需主动展开，离开页面、切换区域或窗口失焦时收起。
- 主持人没有提前查看其他画者答案的特权。猜题者和大屏在揭晓前收不到答案字段的内容。
- 参加本局的其他人提交答案，不计英文大小写和空白。每人猜中一次得 100 分，同时画者得 50 分；重复提交不再得分。
- 所有猜题者猜中或时间到后揭晓答案，主持人点击进入下一轮；最后一轮结束后显示最终积分。
- 答案由内置词库选择，不依赖外部 AI、词典或网络服务。
- 画者断线不会立即终止本轮，可在原会话中恢复；倒计时继续。成员被移出房间会取消当前猜画游戏，避免无效席位继续参与。
- 谁是卧底和骰子猜点数仍在「聚会游戏」分类中，保留原有规则。画板与原游戏保存独立状态。

## 协同主持

原主持人在「参与者」中展开某人的「协同主持权限」，逐项勾选职责。取消勾选撤销该权限。每次更新只修改对应项目，快速勾选不同项目不会互相覆盖。

| 权限 | 可以操作 | 不包含 |
| --- | --- | --- |
| 内容与弹幕管理 | 查看隐藏内容、显示/隐藏、精选、删除内容、结束投票、调整表情/弹幕模式、审核和清理弹幕 | 房间导出、修改容量、开启原图下载权限 |
| 投屏与排队管理 | 切换自由/排队/审核模式、批准/拒绝申请、叫下一位、结束当前投屏 | 其他房间的投屏操作 |
| 游戏与画板主持 | 主持原有小游戏、创建/开始/推进/结束猜画游戏、清空画板 | 提前查看其他人的游戏秘密 |
| 现场大屏控制 | 切换上屏模式、选择投票、控制轮播、在房间内容和投屏间切换 | 分配新的大屏配对会话、房间邀请管理 |

所有管理接口在服务端检查当前权限。权限绑定房间成员身份，只对当前房间生效；不是全局管理员。禁言会暂停该成员的全部协同权限，取消禁言后恢复已授予的权限；移出房间会使原会话失效。只有原主持人可以授权、管理成员、修改全局房间设置、导出、结束或删除房间。

## 保存、部署与限制

- 数据库自动升级到 v7：成员表增加权限列，新增房间画板状态表。已有房间和权限默认行为不变，原参与者默认没有协同管理权。
- 笔画、游戏轮次和权限写入现有 `/data/roomdeck.db`，沿用 `ROOMDECK_DATA_PATH` 映射。随房间删除而清理，无需新增 Compose 服务、端口或环境变量。
- 已确认笔画和授权可在刷新、重连及服务重启后恢复；猜画倒计时按服务端时间计算，重启不重新计时。
- 每个画板最多 600 段笔画，每段最多 128 个点；超过上限需保存图片后清空。前端长笔画每 32 点拆分同步。此容量限制用于约束数据库和浏览器负载。
- 画板可见时约每 800ms 检查版本；没有变化时只返回轻量状态，变化时重新读取画板。隐藏页面停止读取，不因每笔绘画刷新整个房间内容流。本版不是逐点 WebSocket 广播或无限画布，不宣称大规模低延迟协作性能。
- PNG 由浏览器在本地生成。现有房间 ZIP 导出暂不包含画板，画板图片需要单独保存。
- 单实例部署边界保持不变。升级前备份整个数据目录；v7 数据不能直接用旧版应用打开。

## English quick start

Open **Games → Board / Draw & guess** to draw together. Use **Save board image** to download a PNG. Hosts can start a 2–12 player drawing game, with one 90-second turn per player. A correct guess earns 100 points; the artist earns 50. Only the current artist receives the secret word before the reveal.

Under **People → Co-host permissions**, the owner can separately grant content moderation, screen queue management, game hosting and display control. Grants are room-scoped and checked by the server. Muting suspends them. Co-hosts cannot delegate access, change global room settings, export, close or delete a room.

The existing `/data` mount stores boards and permissions. No additional Docker services, ports or configuration are required. Wait for **Synced** before leaving the board; unacknowledged strokes are not guaranteed to survive a reload.
