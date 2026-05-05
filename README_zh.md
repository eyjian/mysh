# mysh

带有语法高亮和智能自动补全的 MySQL 命令行客户端。

在标准 MySQL CLI 体验之上，提供实时 SQL 语法高亮、上下文感知的自动补全和交互式编辑功能。

## 功能特性

- **语法高亮** — 实时着色 SQL 关键字、字符串、数字、注释、函数和运算符
- **智能自动补全** — 根据上下文提供关键字、表名、列名和数据库名建议
- **SQL 代码片段** — 内置 CREATE TABLE、ALTER、INSERT 等模板，Tab 自动展开
- **交互式编辑器** — 支持多行编辑、光标移动、历史导航和选中
- **命令历史** — 持久化历史记录，支持搜索（`Ctrl+R`）、导航（上/下箭头）和去重
- **多种输出格式** — 表格（默认）、垂直格式（`\G`）、JSON（`\j`）和 Markdown（`\m`）结果展示
- **结果导出** — 通过 `\export` 将查询结果导出为 CSV、JSON 或 Markdown 文件
- **实时监控** — 通过 `\watch` 定期重执行查询，实时监控数据变化
- **SQL 别名** — 在配置文件中或交互式定义常用查询的快捷方式
- **会话管理** — 通过 `\session` 保存、切换和删除数据库连接配置
- **Schema 元数据缓存** — 自动缓存表/列信息，DDL 语句后延迟刷新
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
# 使用参数连接
mysh -h 127.0.0.1 -P 3306 -u root -p mydb

# 或使用 DSN 字符串
mysh root:password@tcp(127.0.0.1:3306)/mydb
```

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-h` | `127.0.0.1` | MySQL 主机 |
| `-P` | `3306` | MySQL 端口 |
| `-u` | `root` | MySQL 用户 |
| `-p` | (空) | MySQL 密码 |
| `-d` | (空) | 默认数据库 |
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
| `\help` | 显示帮助 |
| `\connect <dsn>` | 连接数据库 |
| `\reconnect` | 重新连接当前服务器 |
| `\use <db>` | 切换数据库 |
| `\refresh` | 刷新元数据缓存 |
| `\status` | 显示连接状态 |
| `\format table\|vertical\|json\|markdown` | 更改输出格式 |
| `\history [pattern]` | 搜索命令历史 |
| `\source <file>` | 从文件执行 SQL |
| `\export <file> [csv\|json\|markdown]` | 导出最后一次查询结果到文件 |
| `\watch [秒] [SQL]` | 定期重执行查询（默认 5 秒） |
| `\alias [name sql]` | 查看/设置命令别名 |
| `\unalias <name>` | 删除临时别名 |
| `\session` | 列出已保存会话 |
| `\session <name>` | 切换到已保存会话 |
| `\session save <name>` | 保存当前连接为会话 |
| `\session delete <name>` | 删除已保存会话 |
| `\mouse` | 切换鼠标模式 |
| `\quit` | 退出 mysh |

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
│  │ MySQL    │ │ Schema    │ │ 文件   │ │
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
├── connection/          # MySQL 连接池
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
