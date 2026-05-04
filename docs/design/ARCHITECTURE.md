# mysh 架构设计文档

> **项目**：mysh — MySQL CLI + 语法高亮 + 自动补全  
> **架构师**：陈架构（architect）  
> **日期**：2026-05-04  
> **版本**：v1.0

---

## 1. 项目概述

mysh 是一个增强型 MySQL 命令行客户端，在原生 MySQL CLI 基础上提供：

- **语法高亮**：SQL 关键字、字符串、数字、注释等实时着色
- **智能自动补全**：上下文感知的 SQL 关键字、表名、列名补全
- **交互增强**：历史记录搜索、多行编辑、友好提示

目标用户为日常与 MySQL 交互的开发者和 DBA，定位为 **轻量级单机工具**。

---

## 2. 技术选型

| 维度 | 选择 | 理由 |
|------|------|------|
| 语言 | **Go** | 编译为单二进制、无运行时依赖、交叉编译便捷、生态丰富 |
| SQL 解析 | `github.com/xwb1989/sqlparser` | 成熟的 Go SQL 解析器，支持 MySQL 方言 |
| 终端 UI | `github.com/charmbracelet/bubbletea` + `lipgloss` | 现代 TUI 框架，灵活的样式系统 |
| 行编辑 | `github.com/charmbracelet/bubbletea` 自建 | 需要精细控制光标和渲染 |
| 自动补全 | `github.com/charmbracelet/bubbletea` 组件 | 与 TUI 框架一体 |
| MySQL 驱动 | `github.com/go-sql-driver/mysql` | 官方推荐，稳定可靠 |
| 配置 | `github.com/spf13/viper` | 支持 YAML/TOML/ENV 多源配置 |
| 构建 | Go Modules | 标准化依赖管理 |

---

## 3. 系统架构

### 3.1 分层架构

```
┌─────────────────────────────────────────┐
│              CLI Entry (main)            │  命令行入口、参数解析
├─────────────────────────────────────────┤
│           TUI Layer (bubbletea)         │  界面渲染、事件循环
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ Editor   │ │ Completer │ │ Syntax │ │
│  │ Component│ │ Component │ │Highlight│ │
│  └──────────┘ └───────────┘ └────────┘ │
├─────────────────────────────────────────┤
│           Service Layer                  │  业务逻辑
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ SQL Exec │ │ Metadata  │ │ History│ │
│  │ Service  │ │ Service   │ │ Service│ │
│  └──────────┘ └───────────┘ └────────┘ │
├─────────────────────────────────────────┤
│           Data Layer                     │  数据访问
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ MySQL    │ │ Schema    │ │ File   │ │
│  │ Conn Pool│ │ Cache     │ │ Store  │ │
│  └──────────┘ └───────────┘ └────────┘ │
└─────────────────────────────────────────┘
```

### 3.2 模块职责

| 模块 | 包路径 | 职责 |
|------|--------|------|
| **main** | `mysh/main.go` | 入口，初始化配置和依赖，启动 TUI |
| **config** | `mysh/config/` | 加载 ~/.mysh.yaml 配置，管理连接参数 |
| **tui** | `mysh/tui/` | bubbletea Model 定义，主事件循环 |
| **editor** | `mysh/editor/` | 行编辑器，光标移动、多行输入、选择 |
| **highlight** | `mysh/highlight/` | SQL 语法高亮，token 分类与着色 |
| **completer** | `mysh/completer/` | 上下文感知的自动补全引擎 |
| **executor** | `mysh/executor/` | SQL 执行、结果获取、流式输出 |
| **metadata** | `mysh/metadata/` | Schema 元数据缓存（数据库/表/列） |
| **history** | `mysh/history/` | 命令历史记录持久化与搜索 |
| **connection** | `mysh/connection/` | MySQL 连接池管理 |
| **output** | `mysh/output/` | 结果格式化输出（表格/垂直/JSON） |

---

## 4. 核心流程

### 4.1 启动流程

```
main() → config.Load() → connection.New(cfg) → metadata.Refresh()
       → history.Load() → tui.New(deps...) → tea.Program.Run()
```

### 4.2 SQL 输入与执行流程

```
用户按键 → bubbletea.Update()
         ├─ 编辑键 → editor.Update() → 高亮渲染
         ├─ Tab/Ctrl-Space → completer.Suggest(ctx, input)
         │                    → 显示候选列表 → 选择 → 插入
         ├─ Enter → executor.Execute(sql)
         │          → connection.Query(sql)
         │          → output.Format(rows)
         │          → history.Append(sql)
         │          → metadata.ConditionalRefresh()
         └─ Ctrl-C → 取消当前输入/查询
```

### 4.3 自动补全上下文推断

```
输入文本 → sqlparser.Parse() (容错)
        → 提取当前位置的 AST 上下文
        → 判断补全类型：
           ├─ 语句起始 → SQL 关键字（SELECT, INSERT, ...）
           ├─ FROM 后 → 表名/数据库名
           ├─ SELECT 后 / WHERE 后 → 列名
           ├─ JOIN 后 → 表名
           └─ . 后 → 限定的列名
        → 查询 metadata cache
        → 排序返回候选列表
```

---

## 5. 关键设计决策

### 5.1 容错 SQL 解析

自动补全需要解析**不完整**的 SQL。策略：

- 使用 `sqlparser` 的容错模式，提取部分 AST
- 解析失败时，退回到基于正则的启发式上下文推断
- 不要求解析成功才能工作，优雅降级

### 5.2 元数据缓存策略

| 策略 | 说明 |
|------|------|
| 初始加载 | 连接时全量加载当前数据库的表和列 |
| 惰性刷新 | DDL 语句（CREATE/ALTER/DROP）执行后标记脏，下次补全时刷新 |
| 手动刷新 | `\refresh` 命令强制刷新 |
| 按需加载 | 切换数据库时加载新库的元数据 |

### 5.3 语法高亮策略

- 基于 tokenizer 的增量高亮（非完整 AST），实时性好
- Token 类型：Keyword / String / Number / Comment / Identifier / Operator / Function
- 颜色方案可通过配置文件自定义

### 5.4 输出格式

- **表格模式**（默认）：对齐列宽，类似 mysql CLI
- **垂直模式**：`\G` 后缀切换，每行一列
- **JSON 模式**：`\j` 后缀切换，JSON 数组输出

---

## 6. 配置设计

配置文件路径：`~/.mysh.yaml`

```yaml
# 连接默认值
connection:
  host: "127.0.0.1"
  port: 3306
  user: "root"
  password: ""
  database: ""

# 界面
ui:
  prompt: "mysh> "
  multiline_prompt: "    -> "
  page_size: 20          # 结果分页行数

# 高亮主题
theme:
  keyword: "bold magenta"
  string: "yellow"
  number: "cyan"
  comment: "dim"
  function: "green"
  operator: "white"

# 历史
history:
  file: "~/.mysh_history"
  max_entries: 10000

# 补全
completion:
  min_chars: 2           # 最少输入字符数才触发
  max_suggestions: 15
```

---

## 7. 内置命令

| 命令 | 功能 |
|------|------|
| `\help` | 显示帮助 |
| `\connect <dsn>` | 连接数据库 |
| `\use <db>` | 切换数据库 |
| `\refresh` | 刷新元数据缓存 |
| `\status` | 显示连接状态 |
| `\format table\|vertical\|json` | 切换输出格式 |
| `\history [pattern]` | 搜索历史 |
| `\source <file>` | 执行 SQL 文件 |
| `\quit` | 退出 |

---

## 8. 错误处理策略

| 层级 | 策略 |
|------|------|
| 连接层 | 自动重连（最多 3 次），超时 10s |
| 执行层 | 捕获 MySQL error，友好格式输出，不崩溃 |
| 解析层 | 高亮/补全解析失败静默降级，不影响编辑 |
| TUI 层 | panic recovery，保证终端状态恢复 |

---

## 9. 项目目录结构

```
mysh/
├── main.go                    # 入口
├── go.mod
├── go.sum
├── config/
│   └── config.go              # 配置加载
├── connection/
│   └── pool.go                # MySQL 连接池
├── tui/
│   ├── model.go               # bubbletea 主 Model
│   ├── update.go              # Update 逻辑
│   └── view.go                # View 渲染
├── editor/
│   └── editor.go              # 行编辑器
├── highlight/
│   ├── tokenizer.go           # SQL tokenizer
│   └── highlighter.go         # 高亮渲染
├── completer/
│   ├── completer.go           # 补全引擎
│   └── context.go             # 上下文推断
├── executor/
│   └── executor.go            # SQL 执行器
├── metadata/
│   └── cache.go               # Schema 元数据缓存
├── history/
│   └── history.go             # 命令历史
├── output/
│   ├── table.go               # 表格格式
│   ├── vertical.go            # 垂直格式
│   └── json.go                # JSON 格式
├── docs/
│   ├── design/
│   │   ├── ARCHITECTURE.md    # 本文档
│   │   └── data-interfaces.yaml
│   └── delivery/
│       └── checklist.md
└── tests/
    └── ...                    # 集成测试
```

---

## 10. 依赖图

```
main → config, connection, tui, history
tui → editor, highlight, completer, executor, output
completer → metadata, highlight
executor → connection, metadata, history, output
metadata → connection
history → (filesystem)
config → (filesystem)
```

无循环依赖。tui 是核心胶水层，不直接依赖 connection，通过 executor 间接访问。

---

## 11. 可演进方向

- **多方言支持**：抽象 SQL 解析层，支持 PostgreSQL / SQLite
- **插件系统**：Lua/WASM 插件扩展命令
- **结果集操作**：内建对查询结果的过滤/排序/聚合
- **SSH 隧道**：内置 SSH 端口转发连接远程数据库
- **会话管理**：多连接切换
