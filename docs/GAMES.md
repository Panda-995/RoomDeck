# 游戏中心 / Games

两款游戏共用房间成员身份，一房同时一局，最多 12 名玩家。主持人创建并选择词库语言；成员入座后即可开始。主持人可参赛，也可以只控制流程。每个人的界面语言独立切换，词库语言按本局固定。

## 谁是卧底

- 4–12 人，服务端随机选择 1 名卧底，其余为平民。服务端发放相似的两种词；玩家只收到自己的词，不直接知道身份。
- 按座位轮流口头描述，发言者或主持人点击“发言结束”。无人操作就停留在当前阶段，适合现场聚会；没有自动倒计时或内置语音聊天。
- 全体存活玩家秘密投票，不能投自己，提交后锁定。票数最多者淘汰。
- 平票只对并列者复投一次，再平票本轮不淘汰，重新描述。
- 卧底被淘汰则平民获胜；存活卧底数达到存活平民数则卧底获胜。结束后公开词语和身份。
- 主持人取消游戏或有人被移除而导致本局终止时，不公开秘密词语。

内置各 12 组中文/英文词对，后续可扩充。没有白板、多个卧底等变体。大屏和普通房间快照不含私人词语，房主身份本身不能获得其他人的词。

## 骰子猜点数

- 2–12 人，每人秘密选 1–6 中一个点数，提交后不能修改。
- 全体提交后服务器使用密码学随机源掷一枚六面骰，统一公布点数和每个人的猜测。
- 猜中获得 1 分，其他人不扣分。主持人可开始下一轮，积分在本局累计。
- 这是猜单枚骰子的玩法，不是吹牛骰、押注游戏或猜多枚骰子总数。

## 断线、重复操作与数据

游戏状态保存在 `/data/roomdeck.db`；刷新、网络重连和应用重启会保留当前阶段，不会重新发词或重新掷骰。浏览器保留原会话时可回到原座位；清除 Cookie 后不会因为昵称相同就恢复秘密身份。

同一轮的不同玩家可以并发提交。接口同时检查游戏 ID、回合、阶段、复投状态和本人是否已提交，旧轮请求不能进入新轮，重复提交不能产生第二次结算。普通管理操作继续使用版本冲突检查。

大厅玩家被移除会清理其座位；游戏中玩家被移除，下一次状态读取或动作会结束本局而不泄密。临时离线保留座位，主持人可在控制区结束卡住的游戏，再创建新局；没有自动替玩家投票。房间关闭后游戏只读，到期随房间清理。

当前只保留每个房间最近一局，不提供跨局排行榜或历史对局导出。整站备份包含当前游戏状态；现有照片 ZIP 不包含未公开的游戏秘密。

**English:** Undercover supports 4–12 players with one randomly assigned undercover player, private words, spoken turns, secret votes, one runoff and server-side win resolution. Guess the dice supports 2–12 players: everyone locks a guess from 1 to 6, the server rolls once, exact matches earn one point, and the host starts the next round. Both games persist across refresh/restart, preserve private information server-side, reject duplicate/stale actions, and become read-only when the room closes. Clearing cookies does not restore a seat by nickname. One current game is retained per room; no voice chat, betting, historical leaderboard or automatic turn timer is included.
