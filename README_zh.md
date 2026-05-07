# mysh

带有语法高亮和智能自动补全的 MySQL / PostgreSQL 命令行客户端。

在标准 CLI 体验之上，提供实时 SQL 语法高亮、上下文感知的自动补全和交互式编辑功能，同时支持 MySQL 和 PostgreSQL 数据库。

## 功能特性

- **语法高亮** — 实时着色 SQL 关键字、字符串、数字、注释、函数和运算符
- **智能自动补全** — 根据上下文提供关键字、表名、列名和数据库名建议
- **SQL 代码片段** — 内置 CREATE TABLE、ALTER、INSERT 等模板，Tab 自动展开
- **交互式编辑器** — 支持多行编辑、光标移动、历史导航和选中
- **外部编辑器** — 通过 `\edit` 打开 `$EDITOR`（vim/vi）编辑 SQL，保存后自动执行
- **命令历史** — 持久化历史记录，支持搜索（`Ctrl+R`）、导航（上/下箭头）和去重
- **多种输出格式** — 表格（默认）、垂直格式（`\G`）、JSON（`\j`）和 Markdown（`\m`）结果展示
- **结果导出** — 通过 `\export` 将查询结果导出为 CSV、JSON 或 Markdown 文件
- **管道输出** — 通过 `\pipe` 将查询结果管道到系统命令（如 `\pipe grep pattern`）
- **实时监控** — 通过 `\watch` 定期重执行查询，实时监控数据变化
- **查询计时** — 通过 `\timing` 开关查询执行耗时显示
- **NULL 值区分** — NULL 值以灰色斜体显示，与空字符串明确区分
- **SQL 别名** — 在配置文件中或交互式定义常用查询的快捷方式
- **会话管理** — 通过 `\session` 保存、切换和删除数据库连接配置
- **事务支持** — 支持 BEGIN/COMMIT/ROLLBACK 显式事务，专用连接绑定，`tx>` 提示符指示，退出自动回滚
- **Schema 元数据缓存** — 自动缓存表/列信息，DDL 语句后延迟刷新
- **PostgreSQL 支持** — 通过 `--driver postgres` 或 `postgres://` DSN 连接 PostgreSQL 数据库
- **可配置主题** — 通过 `~/.mysh.yaml` 自定义配色方案
- **零运行时依赖** — 单个静态二进制文件，无需 CGO

## 安装

### 一键安装（推荐）

```bash
curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
```

指定安装目录：

```bash
INSTALL_DIR=~/.local/bin curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
```

### go install

```bash
go install github.com/eyjian/mysh@latest
```

### 手动下载

从 [GitHub Releases](https://github.com/eyjian/mysh/releases) 下载最新二进制文件：

| 系统 | 架构 | 文件 |
|------|------|------|
| Linux | amd64 | `mysh-linux-amd64` |
| Linux | arm64 | `mysh-linux-arm64` |
| macOS | amd64 | `mysh-darwin-amd64` |
| macOS | arm64 | `mysh-darwin-arm64` |
| Windows | amd64 | `mysh-windows-amd64.exe` |
| Windows | arm64 | `mysh-windows-arm64.exe` |

```bash
# Linux amd64 示例
curl -sfL -o /usr/local/bin/mysh https://github.com/eyjian/mysh/releases/latest/download/mysh-linux-amd64
chmod +x /usr/local/bin/mysh
```

### 从源码构建

```bash
git clone https://github.com/eyjian/mysh.git
cd mysh
make build
```

## 使用方法

### 基本连接

```bash
# 连接 MySQL（默认）
mysh -h 127.0.0.1 -P 3306 -u root -p mydb

# 连接 PostgreSQL
mysh --driver postgres -h 127.0.0.1 -P 5432 -u postgres -p mydb

# 或使用 DSN 字符串
mysh root:password@tcp(127.0.0.1:3306)/mydb           # MySQL
mysh postgres://postgres:password@127.0.0.1:5432/mydb  # PostgreSQL
```

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--driver` | `mysql` | 数据库驱动（`mysql` 或 `postgres`） |
| `-h` | `127.0.0.1` | 数据库主机 |
| `-P` | `3306`/`5432` | 数据库端口（MySQL 默认 3306，PostgreSQL 默认 5432） |
| `-u` | `root` | 数据库用户 |
| `-p` | (空) | 数据库密码 |
| `-d` | (空) | 默认数据库 |
| `-N` | 关 | 不在结果中显示列名 |
| `--config` | `~/.mysh.yaml` | 配置文件路径 |
| `--version` | — | 打印版本号 |
| `--help` | — | 打印帮助信息 |

### 交互式使用

连接后，输入 SQL 语句并按 `Enter` 执行：

```sql
mysh> SELECT * FROM users WHERE id = 1;
mysh> SHOW TABLES;
mysh> DESCRIBE users;
```

支持多行输入 — 在未结束的语句后按 `Enter` 继续输入：

```sql
mysh> SELECT id, name
    -> FROM users
    -> WHERE age > 18;
```

### 事务支持

mysh 支持显式事务，事务期间使用专用连接绑定：

```sql
mysh> BEGIN;
tx> INSERT INTO orders (user_id, amount) VALUES (1, 99.9);
tx> INSERT INTO order_items (order_id, product_id) VALUES (LAST_INSERT_ID(), 42);
tx> COMMIT;
mysh>
```

- 执行 `BEGIN` 后，提示符变为 `tx>`，表示事务正在进行
- 事务内的所有语句在同一个专用连接上执行，确保事务一致性
- 使用 `\rollback` 或 `ROLLBACK` 回滚事务
- 退出时（`\q`、`Ctrl+D`、`Ctrl+C`）如有未提交事务，自动回滚

### 快捷键

| 按键 | 功能 |
|------|------|
| `Tab` / `Ctrl+Space` | 触发自动补全（单匹配时展开代码片段） |
| `Up` / `Down` | 浏览历史记录 |
| `Ctrl+R` | 搜索历史记录 |
| `Ctrl+C` | 取消查询/监控或清除输入 |
| `Ctrl+D` | 退出 mysh |
| `Home` / `Ctrl+A` | 光标移到行首 |
| `End` / `Ctrl+E` | 光标移到行尾 |
| `Ctrl+U` | 删除光标前所有内容 |
| `Ctrl+K` | 删除光标后所有内容 |
| `Ctrl+W` | 删除光标前一个单词 |
| `Left` / `Right` | 移动光标 |
| `PgUp` / `PgDown` | 滚动输出区域 |

### 输出格式

| 后缀 | 格式 | 示例 |
|------|------|------|
| (默认) | 对齐表格 | `SELECT * FROM users;` |
| `\G` | 垂直格式（每列一行） | `SELECT * FROM users\G` |
| `\j` | JSON 数组 | `SELECT * FROM users\j` |
| `\m` | Markdown 表格 | `SELECT * FROM users\m` |

### 导出查询结果

将最后一次查询结果导出到文件：

```sql
mysh> SELECT * FROM users;
mysh> \export ~/users.csv
mysh> \export ~/users.json json
mysh> \export ~/users.md markdown
```

文件格式根据扩展名自动推断（`.csv`、`.json`、`.md`），也可显式指定。

### 实时监控模式

定期重执行查询，实时监控数据变化：

```sql
mysh> \watch 2 SELECT COUNT(*) FROM processes;
mysh> \watch 5                    -- 每 5 秒重执行上一次查询
```

按 `Ctrl+C` 停止监控。

### SQL 别名

为常用查询定义快捷方式：

```sql
mysh> \alias top10 SELECT * FROM users ORDER BY score DESC LIMIT 10
mysh> top10                        -- 自动展开为完整 SQL
```

或在 `~/.mysh.yaml` 中配置：

```yaml
aliases:
  top10: "SELECT * FROM users ORDER BY score DESC LIMIT 10"
  active: "SELECT * FROM users WHERE status = 'active'"
```

### 会话管理

保存和切换多个数据库连接：

```sql
mysh> \session save prod           -- 保存当前连接为 "prod"
mysh> \session save staging        -- 保存另一个为 "staging"
mysh> \session prod                -- 切换到 "prod"
mysh> \session                     -- 列出所有已保存会话
mysh> \session del staging         -- 删除会话
```

### 鼠标模式

默认启用鼠标模式，滚轮控制输出区域滚动。输入 `\mouse` 可切换：

| 模式 | 行为 |
|------|------|
| 鼠标模式开启（默认） | 滚轮滚动输出区域 |
| 鼠标模式关闭 | 支持终端原生文本选中复制 |

## 配置

配置文件：`~/.mysh.yaml`

```yaml
# 连接默认值
connection:
  driver: "mysql"          # "mysql"（默认）或 "postgres"
  host: "127.0.0.1"
  port: 3306
  user: "root"
  password: ""
  database: ""

# UI 设置
ui:
  prompt: "mysh> "
  multiline_prompt: "    -> "
  page_size: 0           # 结果分页行数（0 = 不分页，类似 mysql CLI）

# 语法高亮主题
theme:
  keyword: "bold magenta"
  string: "yellow"
  number: "cyan"
  comment: "dim"
  function: "green"
  operator: "white"

# 历史记录设置
history:
  file: "~/.mysh_history"
  max_entries: 10000

# 自动补全设置
completion:
  min_chars: 2           # 触发补全的最少字符数
  max_suggestions: 15

# SQL 别名
aliases:
  top10: "SELECT * FROM users ORDER BY score DESC LIMIT 10"
  active: "SELECT * FROM users WHERE status = 'active'"

# 保存的会话
sessions:
  prod:
    host: "db.prod.example.com"
    port: 3306
    user: "admin"
    database: "myapp"
  staging:
    host: "db.staging.example.com"
    port: 3306
    user: "dev"
    database: "myapp_dev"
```

主题值使用 [lipgloss](https://github.com/charmbracelet/lipgloss) 样式语法：`bold`、`italic`、`underline`、`dim`，以及颜色名称（`red`、`green`、`yellow`、`blue`、`magenta`、`cyan`、`white`）或十六进制颜色代码（`#ff0000`）。

## 内置命令

| 命令 | 说明 |
|------|------|
| `\alias [name sql]` | 查看/设置命令别名 |
| `\cd [dir]` | 切换/查看工作目录（用于 \source、\sys） |
| `\clear`, `\c` | 清屏 |
| `\connect <dsn>` | 连接数据库（user@host:port/db 或仅 db） |
| `\conninfo` | 显示详细连接信息（主机、端口、用户、驱动） |
| `\copy <what>` | 复制到剪贴板（result, query, sql） |
| `\desc <t> [mode]` | 查看表结构（无参数=列出表；columns, full, indexes, create） |
| `\di [table]`, `\indexes` | 列出索引（可选表名过滤） |
| `\dn`, `\schemas` | 列出模式（MySQL: 数据库，PostgreSQL: schemas） |
| `\dt [pattern]`, `\tables` | 列出表（支持模式匹配：user\* 或 user%） |
| `\du`, `\users` | 列出数据库用户 |
| `\dv [pattern]`, `\views` | 列出视图（支持模式匹配） |
| `\echo <text>` | 输出文本到界面 |
| `\edit`, `\e` | 打开外部编辑器编辑/执行 SQL |
| `\encoding [name]` | 查看/设置客户端字符编码 |
| `\explain [analyze] <sql>` | 执行 EXPLAIN（加 analyze 则实际运行） |
| `\export <file> [fmt]` | 导出最后一次查询结果到文件（csv, json, markdown） |
| `\fav`, `\favorites` | 列出收藏查询 |
| `\fav <name>` | 执行已收藏的查询 |
| `\fav + <name> [desc]` | 将最后一次查询保存为收藏 |
| `\fav - <name>` | 删除收藏 |
| `\fav show <name>` | 查看收藏的 SQL |
| `\format [type]` | 设置/查看输出格式（table, vertical, json, markdown） |
| `\g [file]` | 执行上次查询，可选保存到文件 |
| `\get <name>` | 查看会话变量值 |
| `\gx` | 以垂直格式执行上次查询 |
| `\help`, `\h`, `\?` | 显示帮助 |
| `\history [pattern]` | 搜索/查看命令历史 |
| `\l`, `\list`, `\databases` | 列出所有数据库 |
| `\mouse` | 切换鼠标模式 |
| `\pipe`, `\| <cmd>` | 将查询结果管道到系统命令 |
| `\privileges <t>` | 查看表权限 |
| `\prompt <var> [text]` | 交互式输入（存储到变量） |
| `\pset [opt [val]]` | 控制输出细节。选项：`expanded [on\|off\|auto]`、`format [table\|vertical\|json\|markdown]`、`header [on\|off]`、`null [string]`、`pager`、`title [text\|off]` |
| `\quit`, `\q`, `quit`, `exit` | 退出 mysh |
| `\reconnect` | 重新连接当前服务器 |
| `\refresh`, `\r` | 刷新元数据缓存 |
| `\rollback` | 回滚当前事务 |
| `\safe-updates [on\|off]` | 切换安全更新模式（阻止没有 WHERE/LIMIT 的 UPDATE/DELETE） |
| `\session` | 列出已保存会话 |
| `\session <name>` | 切换到已保存会话 |
| `\session save <name>` | 保存当前连接为会话 |
| `\session del <name>` | 删除已保存会话 |
| `\sf <func>` | 显示函数定义 |
| `\set [name value]` | 查看/设置会话变量 |
| `\slow [seconds]` | 设置/查看慢查询警告阈值（0 = 禁用） |
| `\source <file>` | 从文件执行 SQL |
| `\status`, `\s` | 显示连接状态 |
| `\sys`, `\! <cmd>` | 执行系统命令 |
| `\T [title\|off]` | 设置/清除结果标题 |
| `\timing` | 切换查询执行耗时显示 |
| `\unalias <name>` | 删除临时别名 |
| `\unset <name>` | 删除会话变量 |
| `\use <db>` | 切换数据库 |
| `\verbose` | 切换详细模式（显示完整错误信息） |
| `\warn [on\|off]` | 切换警告显示 |
| `\watch [秒] [SQL]` | 定期重执行查询（默认 5 秒，Ctrl+C 停止） |
| `\x`, `\expanded` | 切换垂直/表格输出模式 |

### 格式后缀

在任何 SQL 语句后追加格式后缀，可覆盖该查询的输出格式：

| 后缀 | 格式 | 示例 |
|------|------|------|
| `\G` | 垂直格式（每列一行） | `SELECT * FROM users\G` |
| `\j` | JSON 数组 | `SELECT * FROM users\j` |
| `\m` | Markdown 表格 | `SELECT * FROM users\m` |

## 自动补全上下文

mysh 根据光标位置提供上下文感知的建议：

| 上下文 | 建议 |
|--------|------|
| 语句开头 | SQL 关键字、内置命令、SQL 代码片段 |
| `SELECT` 之后 | 列名、函数、`DISTINCT`、`*` |
| `FROM` 之后 | 表名、数据库名、`WHERE`/`JOIN` 关键字 |
| `WHERE` / `AND` / `OR` 之后 | 列名、运算符、函数 |
| `JOIN` 之后 | 表名、`ON`/`USING` |
| `table.` 之后 | 该表的列名、`*` |
| `SET` 之后 | 数据库名、`NAMES`/`AUTOCOMMIT` |
| `ORDER BY` / `GROUP BY` 之后 | 列名、`ASC`/`DESC` |

### 内置 SQL 代码片段

当自动补全匹配较少时，会建议代码片段模板。单匹配时按 `Tab` 自动展开：

| 触发词 | 模板 |
|--------|------|
| `CREATE TABLE` | `CREATE TABLE ... (id INT PRIMARY KEY, ...)` |
| `ALTER TABLE` | `ALTER TABLE ... ADD COLUMN ...` |
| `INSERT INTO` | `INSERT INTO ... (...) VALUES (...)` |
| `UPDATE` | `UPDATE ... SET ... WHERE ...` |
| `DELETE FROM` | `DELETE FROM ... WHERE ...` |
| `CREATE INDEX` | `CREATE INDEX ... ON ... (...)` |
| `CREATE USER` | `CREATE USER ... IDENTIFIED BY ...` |
| `GRANT` | `GRANT ... ON ... TO ...` |
| `SELECT INTO` | `SELECT ... INTO OUTFILE ...` |

## 架构

```
┌─────────────────────────────────────────┐
│              CLI 入口 (main)             │
├─────────────────────────────────────────┤
│           TUI 层 (bubbletea)            │
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │  编辑器  │ │  补全器   │ │ 语法   │ │
│  │          │ │           │ │ 高亮   │ │
│  └──────────┘ └───────────┘ └────────┘ │
├─────────────────────────────────────────┤
│             服务层                       │
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ SQL 执行 │ │ 元数据    │ │ 历史   │ │
│  └──────────┘ └───────────┘ └────────┘ │
├─────────────────────────────────────────┤
│             数据层                       │
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ MySQL/PG │ │ Schema    │ │ 文件   │ │
│  │ 连接池   │ │ 缓存      │ │ 存储   │ │
│  └──────────┘ └───────────┘ └────────┘ │
└─────────────────────────────────────────┘
```

详见 [docs/design/ARCHITECTURE.md](docs/design/ARCHITECTURE.md)。

## 开发

### 前置要求

- Go 1.21+

### 构建与测试

```bash
# 构建
make build

# 运行测试
make test

# 详细测试输出
make test-verbose

# 代码检查
make lint

# 交叉编译所有平台
make cross-compile
```

### 项目结构

```
mysh/
├── main.go              # 入口
├── config/              # 配置加载
├── connection/          # 数据库连接池 & 适配器
├── tui/                 # bubbletea TUI 模型
├── editor/              # 行编辑器
├── highlight/           # SQL 分词器 & 高亮器
├── completer/           # 自动补全引擎
├── executor/            # SQL 执行
├── metadata/            # Schema 元数据缓存
├── history/             # 命令历史
├── output/              # 结果格式化
└── docs/design/         # 架构 & 接口文档
```

### 贡献

1. Fork 本仓库
2. 创建功能分支（`git checkout -b feature/my-feature`）
3. 提交更改（`git commit -m 'Add my feature'`）
4. 确保测试通过（`make test`）
5. 推送到分支（`git push origin feature/my-feature`）
6. 发起 Pull Request

## 许可证

[Apache License 2.0](LICENSE)
