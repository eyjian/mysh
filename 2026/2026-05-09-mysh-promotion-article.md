# mysh：一款带语法高亮的 MySQL / PostgreSQL 命令行客户端，由数字人团队开发

> 开源地址：<https://github.com/eyjian/mysh>
> 当前版本：v0.6.8（已发布 6 大平台预编译二进制）
> 适用数据库：MySQL、PostgreSQL、TDSQL（通过 MySQL 协议兼容）
> 许可：Apache-2.0

---

## 一、用命令行的人，都熟悉这些痛

很多用过 `mysql` / `psql` 原生命令行的人，都有过类似的体验：

- **看不见结构**：一长条无高亮的 SQL 糊在终端里，`SELECT`、字符串、数字、注释一个颜色，写错了也不容易发现
- **补全弱鸡**：原生 `mysql` 默认连关键字补全都不开，更别说表名、字段名
- **多行编辑反人类**：粘贴一段 50 行的 SQL 进来，光标就像被绑住了，改个 `WHERE` 都要按几十次方向键
- **结果格式僵化**：要导 CSV？另外开个客户端。要 JSON？写脚本。要 Markdown 贴到周报？手工排版
- **历史命令难找**：上下箭头一条条翻，翻到天荒地老
- **跨数据库要换工具**：MySQL 用 `mysql`，PostgreSQL 用 `psql`，命令习惯还不一样

有没有一款命令行客户端，能既兼顾 MySQL 又兼顾 PostgreSQL，体验向 IDE 看齐，启动还像 cli 一样轻？

`mysh` 就是为这个问题做的。

---

## 二、一眼看见的差异

直接上对比。同样一句

```sql
SELECT id, name FROM users WHERE status = 'active' ORDER BY id DESC LIMIT 10;
```

在原生 `mysql` 客户端里：所有词一个颜色，关键字、字符串、数字、注释全部混在一起，写错了不容易第一时间看出来。

在 mysh 里则会被实时拆色：

| 元素 | 默认配色 |
|------|---------|
| 关键字（`SELECT` / `FROM` / `WHERE` / `ORDER BY` / `LIMIT` …） | 粗体洋红 |
| 字符串（`'active'`） | 黄色 |
| 数字（`10`） | 青色 |
| 注释（`-- ...`、`/* ... */`） | 暗淡灰 |
| 函数（`COUNT(...)`、`NOW()` …） | 绿色 |
| 运算符 | 白色 |

配色全部可在 `~/.mysh.yaml` 中自定义（lipgloss 样式语法，支持加粗、斜体、十六进制颜色）。

输入到一半按 `Tab`，会看到这样的提示：

```
mysh> SELECT * FROM us|
                      └─ 按 Tab 触发字段级补全
                      ┌──────────────────────┐
                      │ users                │
                      │ user_profiles        │
                      │ user_roles           │
                      │ user_login_logs      │
                      └──────────────────────┘

mysh> SELECT * FROM users WHERE us|
                                 ┌──────────────────────┐
                                 │ user_id              │ ← 自动识别表上下文
                                 │ user_name            │   推荐对应字段
                                 │ user_status          │
                                 └──────────────────────┘
```

第二次按 `Tab` 时，`mysh` 已经从 schema 元数据缓存里读出当前数据库 / schema 的表名和列名，**直接补到字段级**——这是原生 cli 完全没有的能力。

---

## 三、核心亮点速览

挑 5 个最能体现"用了回不去"的功能：

### 1. 实时语法高亮 + 字段级智能补全

边输入边着色：关键字、字符串、数字、注释、函数、运算符各有专属颜色。`Tab` 触发的补全不仅认识 SQL 关键字，还认识**当前连接里的库 / 表 / 字段**，配合 schema 缓存做到秒级响应；DDL 后还会延迟刷新，避免补全用旧数据。

### 2. 用熟悉的编辑器写 SQL

```sql
mysh> \edit
```

直接调起 `$EDITOR`（vim / vi / nano 等），写完保存退出，SQL 自动回到 mysh 并执行。再也不用在 cli 里跟一堆 `WHERE` 条件较劲。

### 3. 输出格式想怎么变就怎么变

只换后缀，无需重写：

| 后缀 | 效果 | 适用场景 |
|------|------|---------|
| (默认) | 对齐表格 | 日常查询 |
| `\G` | 垂直格式 | 字段多、列宽爆炸时 |
| `\j` | JSON 数组 | 给同事 / 上下游程序 |
| `\m` | Markdown 表格 | 直接贴周报 / 飞书文档 |

要导出？`\export ~/result.csv`，扩展名自动推断格式（CSV / JSON / Markdown 三选一）。

### 4. 收藏夹、别名、会话管理

```sql
-- 把刚执行的查询收藏起来
mysh> \fav + active_users 前10个活跃用户

-- 下次直接用，SQL 自动回填到输入框，按 Enter 即执行
mysh> \fav active_users
● mysh> SELECT * FROM users WHERE status = 'active' ORDER BY created_at DESC LIMIT 10

-- 切换数据库连接像切 git 分支
mysh> \session save prod
mysh> \session save staging
mysh> \session prod
```

收藏跨会话持久保存在 `~/.mysh_favorites.yaml` 中（最多 1000 条，LRU 自动淘汰），不用担心越攒越多。

### 5. 监控、管道、时间统计一应俱全

```sql
mysh> \watch 2 SELECT COUNT(*) FROM jobs WHERE status='running';
-- 每 2 秒重执行，按 Ctrl+C 停

mysh> SELECT * FROM logs;
mysh> \pipe grep '2025-03-27 09:31:21'   -- 含空格的模式用引号包裹
mysh> \pipe wc -l                         -- 统计结果行数

mysh> \timing                             -- 开关查询耗时显示
```

外加：NULL 值灰色斜体显示（不再和空字符串傻傻分不清）、显式事务支持（`BEGIN` 后提示符变 `tx>`）、退出时未提交事务自动回滚 ……

---

## 四、全功能一览

mysh 内置 **30+ 个反斜杠命令**，按用途分组如下：

```
┌────────────────────────┬─────────────────────────────────────────────┐
│ 连接与会话             │ \connect  \conninfo  \session  \cd          │
├────────────────────────┼─────────────────────────────────────────────┤
│ 元数据与查看           │ \l  \dt  \desc  \dt user*                   │
├────────────────────────┼─────────────────────────────────────────────┤
│ 输出与导出             │ \G  \j  \m  \export  \pipe  \copy           │
├────────────────────────┼─────────────────────────────────────────────┤
│ 编辑与执行             │ \edit  \source  \sys  \!                    │
├────────────────────────┼─────────────────────────────────────────────┤
│ 监控与调试             │ \watch  \timing  \slow  \safe-updates       │
├────────────────────────┼─────────────────────────────────────────────┤
│ 会话变量（兼容 psql）  │ \set  \echo  \prompt                        │
├────────────────────────┼─────────────────────────────────────────────┤
│ 别名 / 收藏 / 历史     │ \alias  \fav  Ctrl+R 历史搜索               │
├────────────────────────┼─────────────────────────────────────────────┤
│ 事务                   │ BEGIN / COMMIT / \rollback                  │
├────────────────────────┼─────────────────────────────────────────────┤
│ 显示控制               │ \pset  \mouse  \clear  \help                │
└────────────────────────┴─────────────────────────────────────────────┘
```

跨数据库支持是通过适配层做的：

```
   用户输入
      │
      ▼
┌───────────────────────────────────────┐
│  mysh 核心（TUI / 高亮 / 补全 / 元数据） │
└──────────────┬────────────────────────┘
               │ DBAdapter 接口
       ┌───────┴────────┐
       ▼                ▼
   MySQL Adapter   PostgreSQL Adapter
       │                │
       │ ←── TDSQL ─────┘ （MySQL 协议兼容，无需改一行代码）
       │
   生产 / 测试 / 开发数据库
```

一个二进制，三种数据库。MySQL DBA、PG DBA、用 TDSQL 的腾讯云用户，都能用同一套命令习惯切来切去。

---

## 五、5 分钟上手

### 安装（一条命令）

```bash
curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
```

或者从 [Releases](https://github.com/eyjian/mysh/releases) 直接下载对应平台的二进制（Linux / macOS / Windows × amd64 / arm64 共 6 份）。无需 CGO，扔到 `/usr/local/bin/` 就能跑。

### 连接

```bash
# MySQL（默认）
mysh -h 127.0.0.1 -P 3306 -u root -p mydb

# PostgreSQL
mysh --driver postgres -h 127.0.0.1 -P 5432 -u postgres -p mydb

# TDSQL —— 用 MySQL 协议兼容，参数和连 MySQL 完全一样
mysh -h gz-tdsql-xxxx.sql.tencentcdb.com -P 3306 -u root -p mydb

# 也支持 DSN 一把梭
mysh root:password@tcp(127.0.0.1:3306)/mydb
mysh postgres://postgres:password@127.0.0.1:5432/mydb
```

### 试试这几条

```sql
-- 看一眼数据库结构
mysh> \dt
mysh> \desc users full

-- 写个查询，注意输入时的高亮和 Tab 补全
mysh> SELECT id, name FROM users WHERE status = 'active' LIMIT 10;

-- 觉得有用？收藏它
mysh> \fav + active_users

-- 把结果导成 Markdown 贴周报
mysh> \export ~/this_week.md markdown

-- 想监控某张表的增长
mysh> \watch 5 SELECT COUNT(*) FROM orders;

-- 想用熟悉的 vim 快捷键写复杂 SQL
mysh> \edit
```

5 分钟够了。

---

## 六、幕后：mysh 是一支数字人研发团队的作品

如果到这里觉得这个工具不错，那么后面这一段可能更值得一读。

**mysh 这个项目，没有人类程序员写过一行代码。**

它来自一个叫 [`ai-rd-team`](https://github.com/eyjian/ai-rd-team) 的开源项目——一个让 AI 主 Agent 派生数字人团队、自主协作完成研发任务的框架。和市面上常见的"AI 代码助手"或"工作流编排"不同，`ai-rd-team` 的定位是：

> 不是"提示词工程"，不是"工作流编排"。
> 是"你搭一个数字人团队，他们自己协作把活干完"。

工作模式大致是这样：

```
        你（在 IDE 里下一句需求）
                │
                │  ai-rd-team run "需求描述"
                ▼
   ┌─────────────────────────────────────┐
   │  TeamEnvironmentManager（Python）    │
   │   · 装配团队（Lite / Standard / Full）│
   │   · 注入 Skills + Memory             │
   │   · 成本追踪 + Hook + 安全约束       │
   └──────────────┬──────────────────────┘
                  │
          ┌───────┴────────┐
          ▼                ▼
     主 Agent ─── 派生 ───→ 子 Agent 团队
   (CodeBuddy 等)         · 周立项 (PM)
                          · 沈需求 (Analyst)
                          · 陈架构 (Architect)
                          · 林开发 (Developer × N)
                          · 王检视 (Reviewer)
                          · 赵测试 (Tester)
                          · 钱运维 (DevOps)
```

每个数字人成员有自己的**角色 / 技能（Skills） / 记忆（Memory）**，成员之间 P2P 通信、自主决策、协作产出文件。开发者要做的，是把需求讲清楚，然后看着 Web 面板等他们完工。

`mysh` 是这套框架打磨出来的真实工程产物之一，迭代数据如下：

| 维度 | 数据 |
|------|------|
| 公开版本 | 从 v0.0.1 一路迭代到 **v0.6.8**，累计 **15 次 Release** |
| 数据库支持 | MySQL、PostgreSQL（v0.6.5 引入 DBAdapter 模式新增） |
| 内置命令 | **30+ 个反斜杠命令**，覆盖连接 / 元数据 / 输出 / 编辑 / 事务 / 监控 |
| 跨平台二进制 | Linux / macOS / Windows × amd64 / arm64 = **6 份预编译产物** |
| 全部由 | ai-rd-team 数字人团队设计、编写、测试、写文档、打 Release |

值得一提的几个版本节点：

- **v0.6.2**：补全 README 全部内置命令清单
- **v0.6.5**：通过 `DBAdapter` 适配层新增 PostgreSQL 支持，一份核心代码同时服务两种数据库
- **v0.6.6**：PostgreSQL DSN 修复、`\pset` 改进
- **v0.6.7**：实现完整的 `\set` 命令（变量替换 + 特殊变量）+ 多行粘贴支持
- **v0.6.8**：收藏夹持久化（独立 YAML + LRU 淘汰）、字段级补全修复、`\pipe` 引号参数支持

这些不是 demo，是真实跑在生产 DBA 工作流里的功能。它们证明了一件事——**自主驱动的数字人研发团队，已经能持续产出真实可用的工程级软件**。

---

## 七、上手 & 反馈

两个仓库都已开源，欢迎试用：

- **mysh**（命令行客户端）
  <https://github.com/eyjian/mysh>
  一键安装：
  ```bash
  curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
  ```

- **ai-rd-team**（背后的数字人研发团队框架）
  <https://github.com/eyjian/ai-rd-team>

如果觉得 mysh 顺手，给个 Star 是最大的鼓励；遇到问题或有想要的功能，欢迎到 GitHub 提 Issue / PR——一部分会由 ai-rd-team 的数字人成员接手处理。

---

> 如果对"AI 数字人团队怎么真正把工程做出来"感兴趣，欢迎关注后续文章。
> 一篇专门讲 ai-rd-team 方法论（NEXT.md 交接、openspec 设计文档、BaseAdapter 多平台适配……）的实战篇，正在路上。
