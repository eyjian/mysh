# OpenSpec: Phase 4 - 高级功能

## 元数据
- **ID**: mysh-phase4-advanced-features
- **版本**: 1.0
- **状态**: implemented
- **作者**: AI Assistant
- **创建日期**: 2026-05-05

## 概述

mysh 第四阶段增强，聚焦 5 个高级功能，将 mysh 从基础查询工具提升为专业级数据库工作台：

1. **查询结果导出** — `\export` 命令将结果集导出为 CSV/JSON/Markdown 文件
2. **`\watch` 实时监控** — 定期执行 SQL 并显示结果变化
3. **SQL 别名系统** — 用户自定义命令别名，简化常用操作
4. **会话管理** — 多连接配置保存/加载/切换
5. **智能 SQL 提示** — 基于上下文的 SQL 片段提示与语句模板

## 任务列表

---

### 任务 1: 查询结果导出

**目标**: 支持 `\export` 命令，将最后一次查询结果导出到文件，支持 CSV、JSON、Markdown 三种格式。无需重新执行查询，直接从内存中的结果集导出。

**设计**:

#### 命令语法

```
\export <文件路径>                  # 根据文件扩展名自动推断格式
\export <文件路径> csv              # 指定 CSV 格式
\export <文件路径> json             # 指定 JSON 格式
\export <文件路径> markdown         # 指定 Markdown 格式
\export                             # 显示用法
```

扩展名推断规则：
- `.csv` → CSV
- `.json` → JSON
- `.md` → Markdown
- 无扩展名或未知 → 报错提示指定格式

#### 新增 Model 状态 (tui/model.go)

```go
// 最后一次查询结果，用于导出
lastResult *executor.QueryResult
lastFormat output.Format
```

在 `displayQueryResult()` 中更新：

```go
func (m *Model) displayQueryResult(result *executor.QueryResult, err error, formatOverride output.Format) {
    // ... 现有逻辑 ...
    
    // 保存最后一次成功的结果，用于导出
    if err == nil && result != nil && result.IsQuery {
        m.lastResult = result
        m.lastFormat = outFmt
    }
}
```

#### 导出功能实现 (output/export.go)

新建 `output/export.go`，包含三种格式的导出函数：

```go
package output

import (
    "encoding/csv"
    "encoding/json"
    "fmt"
    "os"
    "strings"
    
    "mysh/executor"
)

// ExportFormat 表示导出文件格式
type ExportFormat int

const (
    ExportCSV ExportFormat = iota
    ExportJSON
    ExportMarkdown
)

// ParseExportFormat 从字符串解析导出格式
func ParseExportFormat(s string) (ExportFormat, error) {
    switch strings.ToLower(s) {
    case "csv":
        return ExportCSV, nil
    case "json":
        return ExportJSON, nil
    case "markdown", "md":
        return ExportMarkdown, nil
    default:
        return ExportCSV, fmt.Errorf("未知的导出格式: %s (支持: csv, json, markdown)", s)
    }
}

// InferExportFormat 根据文件扩展名推断导出格式
func InferExportFormat(path string) (ExportFormat, error) {
    if strings.HasSuffix(strings.ToLower(path), ".csv") {
        return ExportCSV, nil
    }
    if strings.HasSuffix(strings.ToLower(path), ".json") {
        return ExportJSON, nil
    }
    if strings.HasSuffix(strings.ToLower(path), ".md") {
        return ExportMarkdown, nil
    }
    return ExportCSV, fmt.Errorf("无法从文件扩展名推断格式，请显式指定: %s", path)
}

// ExportResult 将查询结果导出到文件
func ExportResult(result *executor.QueryResult, path string, format ExportFormat) (int, error) {
    f, err := os.Create(path)
    if err != nil {
        return 0, fmt.Errorf("无法创建文件: %w", err)
    }
    defer f.Close()
    
    switch format {
    case ExportCSV:
        return exportCSV(result, f)
    case ExportJSON:
        return exportJSON(result, f)
    case ExportMarkdown:
        return exportMarkdown(result, f)
    default:
        return exportCSV(result, f)
    }
}

// exportCSV 导出为 CSV 格式
func exportCSV(result *executor.QueryResult, w io.Writer) (int, error) {
    writer := csv.NewWriter(w)
    defer writer.Flush()
    
    // 写入表头
    if err := writer.Write(result.Columns); err != nil {
        return 0, err
    }
    
    // 写入数据行
    for _, row := range result.Rows {
        record := make([]string, len(row))
        for i, val := range row {
            record[i] = formatValue(val)
        }
        if err := writer.Write(record); err != nil {
            return 0, err
        }
    }
    
    writer.Flush()
    return len(result.Rows), writer.Error()
}

// exportJSON 导出为 JSON 格式（无 ANSI 颜色码）
func exportJSON(result *executor.QueryResult, w io.Writer) (int, error) {
    records := make([]map[string]interface{}, 0, len(result.Rows))
    for _, row := range result.Rows {
        record := make(map[string]interface{}, len(result.Columns))
        for i, col := range result.Columns {
            if i < len(row) {
                record[col] = convertForJSON(row[i])
            } else {
                record[col] = nil
            }
        }
        records = append(records, record)
    }
    
    encoder := json.NewEncoder(w)
    encoder.SetIndent("", "  ")
    if err := encoder.Encode(records); err != nil {
        return 0, fmt.Errorf("JSON 编码失败: %w", err)
    }
    
    return len(result.Rows), nil
}

// exportMarkdown 导出为 Markdown 表格格式（无 ANSI 颜色码）
func exportMarkdown(result *executor.QueryResult, w io.Writer) (int, error) {
    // 计算列宽
    widths := make([]int, len(result.Columns))
    for i, col := range result.Columns {
        widths[i] = utf8.RuneCountInString(col)
    }
    for _, row := range result.Rows {
        for i, val := range row {
            if i < len(widths) {
                w := utf8.RuneCountInString(formatValue(val))
                if w > widths[i] {
                    widths[i] = w
                }
            }
        }
    }
    
    // 表头
    fmt.Fprint(w, "|")
    for i, col := range result.Columns {
        fmt.Fprintf(w, " %s |", padRight(col, widths[i]))
    }
    fmt.Fprintln(w)
    
    // 分隔行
    fmt.Fprint(w, "|")
    for _, w := range widths {
        fmt.Fprintf(w, " %s |", strings.Repeat("-", w))
    }
    fmt.Fprintln(w)
    
    // 数据行
    for _, row := range result.Rows {
        fmt.Fprint(w, "|")
        for i, val := range row {
            if i < len(widths) {
                fmt.Fprintf(w, " %s |", padRight(formatValue(val), widths[i]))
            }
        }
        fmt.Fprintln(w)
    }
    
    return len(result.Rows), nil
}
```

#### 命令注册 (tui/model.go)

在 `handleBackslashCommand` 中添加：

```go
case "\\export":
    if len(parts) < 2 {
        m.addOutput("用法: \\export <文件路径> [csv|json|markdown]")
        m.addOutput("  扩展名自动推断: .csv, .json, .md")
        m.addOutput("  导出最后一次查询结果")
    } else {
        m.handleExport(parts[1:])
    }
```

```go
// handleExport 处理 \export 命令
func (m *Model) handleExport(parts []string) {
    if m.lastResult == nil {
        m.addOutput("没有可导出的查询结果。请先执行一条 SELECT 查询。")
        return
    }
    
    filePath := parts[0]
    // 展开 ~ 为用户主目录
    if strings.HasPrefix(filePath, "~") {
        home, err := os.UserHomeDir()
        if err == nil {
            filePath = home + filePath[1:]
        }
    }
    
    var format output.ExportFormat
    if len(parts) >= 2 {
        // 显式指定格式
        f, err := output.ParseExportFormat(parts[1])
        if err != nil {
            m.addOutput(fmt.Sprintf("错误: %s", err))
            return
        }
        format = f
    } else {
        // 从扩展名推断
        f, err := output.InferExportFormat(filePath)
        if err != nil {
            m.addOutput(fmt.Sprintf("错误: %s", err))
            return
        }
        format = f
    }
    
    rowCount, err := output.ExportResult(m.lastResult, filePath, format)
    if err != nil {
        m.addOutput(fmt.Sprintf("导出失败: %s", err))
        return
    }
    
    m.addOutput(fmt.Sprintf("已导出 %d 行到 %s (%s 格式)", rowCount, filePath, format))
}
```

#### 帮助文本更新

在 `helpText()` 中添加：
```
  \export <file> [fmt]  Export last result to file (csv/json/markdown)
```

**复杂度**: 中  
**涉及文件**: `output/export.go`（新建）, `tui/model.go`, `output/output.go`（formatValue 需导出）

---

### 任务 2: `\watch` 实时监控

**目标**: 实现 `\watch <秒数>` 命令，定期重新执行上一次的 SQL 查询并在终端实时显示结果，直到用户按 Ctrl+C 停止。适用于监控慢查询、观察数据变化等场景。

**设计**:

#### 命令语法

```
\watch 2                # 每 2 秒执行一次上一次查询
\watch                  # 默认每 5 秒
\watch 10 SELECT 1      # 每 10 秒执行指定 SQL
```

#### 新增 Model 状态 (tui/model.go)

```go
// Watch 模式状态
watching    bool          // true = 正在监控模式
watchInterval time.Duration // 监控间隔
watchQuery  string        // 监控执行的 SQL
watchFormat output.Format // 监控输出格式
```

#### 消息类型 (tui/model.go)

```go
// watchTickMsg 定时触发监控查询
type watchTickMsg time.Time

// watchResultMsg 监控查询结果
type watchResultMsg struct {
    result *executor.QueryResult
    err    error
    tick   int // 第几次执行（用于显示计数）
}
```

#### 命令注册

```go
case "\\watch":
    if m.executing || m.watching {
        m.addOutput("已有查询或监控正在执行。")
        return m, nil
    }
    return m.handleWatch(parts[1:])
```

#### 实现逻辑 (tui/model.go)

```go
// handleWatch 处理 \watch 命令
func (m Model) handleWatch(args []string) (tea.Model, tea.Cmd) {
    // 解析间隔
    interval := 5 * time.Second
    if len(args) >= 1 {
        if sec, err := fmt.Sscanf(args[0], ""); err == nil {
            // 尝试解析数字
            var seconds int
            if _, err := fmt.Sscanf(args[0], "%d", &seconds); err == nil && seconds > 0 {
                interval = time.Duration(seconds) * time.Second
            }
        }
    }
    
    // 确定 SQL：参数中指定 或 使用上一次
    query := ""
    if len(args) >= 2 {
        query = strings.Join(args[1:], " ")
    } else if m.lastResult != nil {
        // 使用上次保存的查询
        query = m.lastQuery
    }
    
    if query == "" {
        m.addOutput("用法: \\watch [秒数] [SQL语句]")
        m.addOutput("  没有上一次查询可监控，请指定 SQL 语句。")
        return m, nil
    }
    
    m.watching = true
    m.watchInterval = interval
    m.watchQuery = query
    m.watchFormat = m.deps.Formatter.CurrentFormat()
    
    m.addOutput(fmt.Sprintf("监控中 (每 %s 执行一次, Ctrl+C 停止)...", interval))
    
    // 立即执行第一次
    return m, tea.Batch(
        m.executeWatchQuery(1),
        watchTickCmd(interval),
    )
}

func watchTickCmd(interval time.Duration) tea.Cmd {
    return tea.Tick(interval, func(t time.Time) tea.Msg {
        return watchTickMsg(t)
    })
}

func (m Model) executeWatchQuery(tick int) tea.Cmd {
    query := m.watchQuery
    return func() tea.Msg {
        result, err := m.deps.Executor.Execute(context.Background(), query+";")
        return watchResultMsg{result, err, tick}
    }
}
```

#### Update 处理

```go
case watchTickMsg:
    if m.watching {
        m.watchTick++
        return m, m.executeWatchQuery(m.watchTick)
    }
    return m, nil

case watchResultMsg:
    if !m.watching {
        return m, nil
    }
    // 清空输出区，只显示最新结果
    m.output = nil
    m.scrollOffset = 0
    m.addOutput(fmt.Sprintf("\033[2m--- Watch #%d (%s) ---\033[0m", msg.tick, time.Now().Format("15:04:05")))
    
    if msg.err != nil {
        m.addOutput(fmt.Sprintf("错误: %s", msg.err))
    } else {
        m.displayQueryResult(msg.result, msg.err, m.watchFormat)
    }
    
    // 继续下一轮定时
    return m, watchTickCmd(m.watchInterval)
```

Ctrl+C 退出监控：

```go
case tea.KeyCtrlC:
    if m.watching {
        m.watching = false
        m.watchQuery = ""
        m.addOutput("监控已停止。")
        return m, nil
    }
    // ... 现有的 Ctrl+C 逻辑
```

还需要在 Model 中保存上一次执行的 SQL：

```go
lastQuery string // 上一次执行的 SQL（供 \watch 和 \export 使用）
```

在 `executeInput` 中更新 `lastQuery`。

**复杂度**: 中  
**涉及文件**: `tui/model.go`

---

### 任务 3: SQL 别名系统

**目标**: 允许用户在配置文件中定义 SQL 别名，用短命令替代常用 SQL 语句。类似于 bash 的 alias 功能，大幅提升日常操作效率。

**设计**:

#### 配置格式 (~/.mysh.yaml)

```yaml
# SQL 别名
aliases:
  dt: "SHOW TABLES"
  dp: "SHOW PROCESSLIST"
  ds: "SHOW STATUS"
  dv: "SHOW VARIABLES"
  de: "SHOW ENGINE INNODB STATUS"
  slow: "SELECT * FROM information_schema.PROCESSLIST WHERE TIME > 5 ORDER BY TIME DESC"
  locks: "SELECT * FROM information_schema.INNODB_LOCKS"
  size: "SELECT table_name, ROUND(data_length/1024/1024,2) AS 'Data(MB)', ROUND(index_length/1024/1024,2) AS 'Index(MB)' FROM information_schema.TABLES WHERE table_schema = DATABASE()"
```

#### 配置结构扩展 (config/config.go)

```go
type Config struct {
    Connection ConnectionConfig
    UI         UIConfig
    Theme      ThemeConfig
    History    HistoryConfig
    Completion CompletionConfig
    Aliases    map[string]string `mapstructure:"aliases"` // 新增
}
```

#### 别名管理命令

```
\alias                    # 显示所有已定义的别名
\alias <名称> <SQL>       # 临时添加别名（仅本次会话有效）
\unalias <名称>           # 删除临时别名
```

#### 新增 Model 状态 (tui/model.go)

```go
tempAliases map[string]string // 本次会话临时添加的别名
```

#### 别名解析逻辑

在 `handleEnter()` 中，在检查反斜杠命令之前，先检查别名：

```go
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
    // ... 现有的补全接受逻辑 ...
    
    input := m.ed.Text()
    trimmed := strings.TrimSpace(input)
    
    // 检查别名（优先级低于反斜杠命令，高于 SQL）
    if !strings.HasPrefix(trimmed, "\\") {
        firstWord := strings.Fields(trimmed)[0]
        if sql, ok := m.resolveAlias(firstWord); ok {
            // 替换第一个词为对应的 SQL
            rest := strings.TrimSpace(strings.TrimPrefix(trimmed, firstWord))
            expanded := sql
            if rest != "" {
                expanded = sql + " " + rest
            }
            m.addOutput(fmt.Sprintf("\033[2m→ %s\033[0m", expanded)) // 淡色显示展开结果
            m.ed.Clear()
            return m.executeInput(expanded, output.FormatTable)
        }
    }
    
    // ... 现有的反斜杠命令和 SQL 执行逻辑 ...
}

// resolveAlias 查找别名，优先查找临时别名，再查找配置别名
func (m Model) resolveAlias(name string) (string, bool) {
    // 临时别名优先
    if m.tempAliases != nil {
        if sql, ok := m.tempAliases[name]; ok {
            return sql, ok
        }
    }
    // 配置文件别名
    if m.deps.Config.Aliases != nil {
        if sql, ok := m.deps.Config.Aliases[name]; ok {
            return sql, ok
        }
    }
    return "", false
}
```

#### 命令注册

```go
case "\\alias":
    if len(parts) < 2 {
        // 显示所有别名
        m.listAliases()
    } else if len(parts) == 2 {
        // 显示指定别名
        if sql, ok := m.resolveAlias(parts[1]); ok {
            m.addOutput(fmt.Sprintf("  %s = %s", parts[1], sql))
        } else {
            m.addOutput(fmt.Sprintf("别名 '%s' 未定义", parts[1]))
        }
    } else {
        // 设置临时别名: \alias name SQL...
        name := parts[1]
        sql := strings.Join(parts[2:], " ")
        if m.tempAliases == nil {
            m.tempAliases = make(map[string]string)
        }
        m.tempAliases[name] = sql
        m.addOutput(fmt.Sprintf("别名已设置: %s = %s", name, sql))
    }

case "\\unalias":
    if len(parts) < 2 {
        m.addOutput("用法: \\unalias <名称>")
    } else {
        name := parts[1]
        if m.tempAliases != nil {
            delete(m.tempAliases, name)
        }
        m.addOutput(fmt.Sprintf("已删除临时别名: %s", name))
    }
```

```go
// listAliases 列出所有已定义的别名
func (m Model) listAliases() {
    count := 0
    
    // 配置文件别名
    if m.deps.Config.Aliases != nil {
        for name, sql := range m.deps.Config.Aliases {
            m.addOutput(fmt.Sprintf("  %-15s → %s", name, sql))
            count++
        }
    }
    
    // 临时别名（标注 * 号）
    if m.tempAliases != nil {
        for name, sql := range m.tempAliases {
            m.addOutput(fmt.Sprintf("  %-15s → %s  \033[33m(temp)\033[0m", name, sql))
            count++
        }
    }
    
    if count == 0 {
        m.addOutput("没有定义别名。可在 ~/.mysh.yaml 的 aliases 节添加。")
    } else {
        m.addOutput(fmt.Sprintf("共 %d 个别名（配置文件 + 临时）", count))
    }
}
```

**复杂度**: 中  
**涉及文件**: `config/config.go`, `tui/model.go`

---

### 任务 4: 会话管理

**目标**: 支持保存和加载多个数据库连接配置（会话），在多个环境（开发/测试/生产）间快速切换，无需每次输入完整连接参数。

**设计**:

#### 配置格式 (~/.mysh.yaml)

```yaml
# 会话管理
sessions:
  dev:
    host: "127.0.0.1"
    port: 3306
    user: "dev"
    password: ""       # 建议留空，运行时输入
    database: "myapp_dev"
    charset: "utf8mb4"
  
  test:
    host: "test-db.internal"
    port: 3306
    user: "tester"
    database: "myapp_test"
  
  prod:
    host: "prod-db.internal"
    port: 3307
    user: "readonly"
    database: "myapp_prod"
```

#### 命令语法

```
\session                    # 列出所有已保存的会话
\session <名称>             # 切换到指定会话
\session save <名称>        # 将当前连接保存为会话
\session delete <名称>      # 删除已保存的会话
```

#### 配置结构扩展 (config/config.go)

```go
type Config struct {
    Connection ConnectionConfig
    UI         UIConfig
    Theme      ThemeConfig
    History    HistoryConfig
    Completion CompletionConfig
    Aliases    map[string]string       `mapstructure:"aliases"`
    Sessions   map[string]SessionConfig `mapstructure:"sessions"` // 新增
}

// SessionConfig 保存一个数据库会话的连接配置
type SessionConfig struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    User     string `mapstructure:"user"`
    Password string `mapstructure:"password"`
    Database string `mapstructure:"database"`
    Charset  string `mapstructure:"charset"`
}

// ToConnectionConfig 将 SessionConfig 转换为 ConnectionConfig
func (s SessionConfig) ToConnectionConfig() ConnectionConfig {
    port := s.Port
    if port == 0 {
        port = 3306
    }
    return ConnectionConfig{
        Host:     s.Host,
        Port:     port,
        User:     s.User,
        Password: s.Password,
        Database: s.Database,
        Charset:  s.Charset,
    }
}
```

#### 命令注册 (tui/model.go)

```go
case "\\session":
    if len(parts) < 2 {
        m.listSessions()
    } else {
        switch parts[1] {
        case "save":
            if len(parts) < 3 {
                m.addOutput("用法: \\session save <名称>")
            } else {
                m.saveSession(parts[2])
            }
        case "delete", "rm", "del":
            if len(parts) < 3 {
                m.addOutput("用法: \\session delete <名称>")
            } else {
                m.deleteSession(parts[2])
            }
        default:
            // 切换到指定会话
            m.switchSession(parts[1])
        }
    }
```

#### 实现逻辑 (tui/model.go)

```go
// listSessions 列出所有会话
func (m Model) listSessions() {
    sessions := m.deps.Config.Sessions
    if len(sessions) == 0 {
        m.addOutput("没有保存的会话。使用 \\session save <名称> 保存当前连接。")
        return
    }
    
    currentCfg := m.deps.Config.Connection
    for name, sess := range sessions {
        marker := " "
        // 标记当前连接
        if sess.Host == currentCfg.Host && sess.Port == currentCfg.Port && 
           sess.User == currentCfg.User && sess.Database == currentCfg.Database {
            marker = "\033[32m*\033[0m" // 绿色星号表示当前会话
        }
        m.addOutput(fmt.Sprintf("  %s %-12s %s@%s:%d/%s", marker, name, sess.User, sess.Host, sess.Port, sess.Database))
    }
}

// switchSession 切换到指定会话
func (m *Model) switchSession(name string) {
    sessions := m.deps.Config.Sessions
    if sessions == nil {
        m.addOutput(fmt.Sprintf("会话 '%s' 不存在。", name))
        return
    }
    sess, ok := sessions[name]
    if !ok {
        m.addOutput(fmt.Sprintf("会话 '%s' 不存在。使用 \\session 查看所有会话。", name))
        return
    }
    
    newCfg := sess.ToConnectionConfig()
    
    if m.deps.Pool == nil {
        m.addOutput("无法切换：没有可用的连接池。")
        return
    }
    
    m.addOutput(fmt.Sprintf("切换到会话 '%s': %s@%s:%d/%s ...", name, newCfg.User, newCfg.Host, newCfg.Port, newCfg.Database))
    
    if err := m.deps.Pool.Reset(&newCfg); err != nil {
        m.addOutput(fmt.Sprintf("连接失败: %s", err))
        m.connected = false
        return
    }
    
    m.deps.Config.Connection = newCfg
    
    // 刷新元数据
    if m.deps.Meta != nil {
        m.deps.Meta.MarkDirty()
        go m.deps.Meta.Refresh()
    }
    
    m.connected = true
    m.addOutput("会话切换成功。")
}

// saveSession 将当前连接保存为会话
func (m *Model) saveSession(name string) {
    cfg := m.deps.Config.Connection
    
    if m.deps.Config.Sessions == nil {
        m.deps.Config.Sessions = make(map[string]config.SessionConfig)
    }
    
    m.deps.Config.Sessions[name] = config.SessionConfig{
        Host:     cfg.Host,
        Port:     cfg.Port,
        User:     cfg.User,
        Password: "", // 安全起见不保存密码
        Database: cfg.Database,
        Charset:  cfg.Charset,
    }
    
    // 持久化到配置文件
    if err := config.Save(m.deps.Config); err != nil {
        m.addOutput(fmt.Sprintf("保存失败: %s（会话仅在本次会话有效）", err))
    } else {
        m.addOutput(fmt.Sprintf("会话 '%s' 已保存到配置文件。", name))
    }
}

// deleteSession 删除指定会话
func (m *Model) deleteSession(name string) {
    if m.deps.Config.Sessions == nil {
        m.addOutput("没有保存的会话。")
        return
    }
    
    if _, ok := m.deps.Config.Sessions[name]; !ok {
        m.addOutput(fmt.Sprintf("会话 '%s' 不存在。", name))
        return
    }
    
    delete(m.deps.Config.Sessions, name)
    
    if err := config.Save(m.deps.Config); err != nil {
        m.addOutput(fmt.Sprintf("删除保存失败: %s", err))
    } else {
        m.addOutput(fmt.Sprintf("会话 '%s' 已删除。", name))
    }
}
```

#### 配置持久化 (config/config.go)

新增 `Save` 函数，将配置写回 YAML 文件：

```go
// Save 将配置写回文件
func Save(cfg *Config) error {
    if cfg == nil || cfg.configPath == "" {
        return fmt.Errorf("无法保存：未指定配置文件路径")
    }
    
    data, err := yaml.Marshal(cfg)
    if err != nil {
        return fmt.Errorf("序列化配置失败: %w", err)
    }
    
    if err := os.WriteFile(cfg.configPath, data, 0600); err != nil {
        return fmt.Errorf("写入配置文件失败: %w", err)
    }
    
    return nil
}
```

**复杂度**: 高  
**涉及文件**: `config/config.go`, `tui/model.go`

---

### 任务 5: 智能 SQL 提示

**目标**: 在用户输入 SQL 时，基于上下文提供 SQL 片段提示（snippet suggestions），比如输入 `CREATE` 时提示 `CREATE TABLE ...` 模板，输入 `ALTER` 时提示 `ALTER TABLE ... ADD COLUMN ...`。与现有自动补全不同，片段提示提供的是多行模板，按 Tab 展开后用占位符标记需要填写的部分。

**设计**:

#### 片段定义 (completer/snippets.go)

新建 `completer/snippets.go`：

```go
package completer

// Snippet 表示一个 SQL 代码片段模板
type Snippet struct {
    Trigger     string // 触发关键词
    Description string // 片段描述
    Template    string // 模板文本，${1:placeholder} 为占位符
    Context     string // 适用上下文（"statement", "after_from" 等）
}

// BuiltInSnippets 返回内置的 SQL 片段列表
func BuiltInSnippets() []Snippet {
    return []Snippet{
        {
            Trigger:     "create",
            Description: "CREATE TABLE template",
            Template:    "CREATE TABLE ${1:table_name} (\n  ${2:id} INT PRIMARY KEY AUTO_INCREMENT,\n  ${3:column_name} ${4:VARCHAR(255)}\n)",
            Context:     "statement",
        },
        {
            Trigger:     "alter",
            Description: "ALTER TABLE ADD COLUMN template",
            Template:    "ALTER TABLE ${1:table_name} ADD COLUMN ${2:column_name} ${3:VARCHAR(255)}",
            Context:     "statement",
        },
        {
            Trigger:     "insert",
            Description: "INSERT INTO template",
            Template:    "INSERT INTO ${1:table_name} (${2:columns}) VALUES (${3:values})",
            Context:     "statement",
        },
        {
            Trigger:     "update",
            Description: "UPDATE template",
            Template:    "UPDATE ${1:table_name} SET ${2:column} = ${3:value} WHERE ${4:condition}",
            Context:     "statement",
        },
        {
            Trigger:     "select",
            Description: "SELECT template",
            Template:    "SELECT ${1:columns} FROM ${2:table_name} WHERE ${3:condition}",
            Context:     "statement",
        },
        {
            Trigger:     "delete",
            Description: "DELETE template",
            Template:    "DELETE FROM ${1:table_name} WHERE ${2:condition}",
            Context:     "statement",
        },
        {
            Trigger:     "join",
            Description: "INNER JOIN template",
            Template:    "INNER JOIN ${1:table_name} ON ${2:condition}",
            Context:     "after_from",
        },
        {
            Trigger:     "left",
            Description: "LEFT JOIN template",
            Template:    "LEFT JOIN ${1:table_name} ON ${2:condition}",
            Context:     "after_from",
        },
        {
            Trigger:     "index",
            Description: "CREATE INDEX template",
            Template:    "CREATE INDEX ${1:index_name} ON ${2:table_name} (${3:column})",
            Context:     "statement",
        },
    }
}
```

#### 补全逻辑扩展 (completer/completer.go)

在 `Complete()` 方法中，当关键词补全和对象补全结果较少时，额外检查是否有匹配的片段：

```go
// 在 Complete() 返回结果前，检查片段匹配
func (c *Completer) Complete(ctx context.Context, input string, cursorPos int) ([]Suggestion, error) {
    // ... 现有的关键词/对象补全逻辑 ...
    
    // 追加片段提示
    if len(suggestions) == 0 || len(suggestions) < 3 {
        word := extractWordAtCursor(input, cursorPos)
        if word != "" {
            for _, snippet := range BuiltInSnippets() {
                if strings.HasPrefix(strings.ToLower(snippet.Trigger), strings.ToLower(word)) && 
                   strings.ToLower(snippet.Trigger) != strings.ToLower(word) {
                    // 同名关键词已存在时跳过（避免重复）
                    found := false
                    for _, s := range suggestions {
                        if strings.EqualFold(s.Text, snippet.Trigger) {
                            found = true
                            break
                        }
                    }
                    if !found {
                        suggestions = append(suggestions, Suggestion{
                            Text:   snippet.Trigger,
                            Type:   SuggestKeyword,
                            Detail: "⟡ " + snippet.Description,
                        })
                    }
                }
            }
        }
    }
    
    return suggestions, nil
}
```

#### 片段展开逻辑 (tui/model.go)

当用户接受一个补全项时，如果该项对应一个片段模板，则展开模板并定位到第一个占位符：

```go
// 在 handleTab 接受补全时
func (m Model) handleTab() (tea.Model, tea.Cmd) {
    // ... 现有逻辑 ...
    
    // 如果只一个匹配，检查是否为片段
    if len(suggestions) == 1 {
        if snippet := findSnippet(suggestions[0].Text); snippet != nil {
            m.ed.SetText(snippet.Template) // 先简单展开为完整模板
            m.ed.MoveEnd()
            m.showComp = false
            return m, nil
        }
        // 原有逻辑
        m.ed.ReplaceWordBeforeCursor(suggestions[0].Text)
        m.showComp = false
        return m, nil
    }
    // ...
}
```

第一阶段实现简化版：片段模板中的 `${N:default}` 占位符暂不做 Tab 跳转，直接展开为默认值。后续版本可增强。

**复杂度**: 低-中  
**涉及文件**: `completer/snippets.go`（新建）, `completer/completer.go`, `tui/model.go`

---

## 实施顺序

1. **任务 1** — 查询结果导出（独立功能，用户体验提升显著）
2. **任务 2** — `\watch` 实时监控（DBA 常用功能，依赖任务 1 的 `lastResult` 基础设施）
3. **任务 3** — SQL 别名系统（轻量功能，提升日常效率）
4. **任务 5** — 智能 SQL 提示（纯补全增强，独立实现）
5. **任务 4** — 会话管理（最复杂，需要配置持久化）

## 总结

| 任务 | 功能 | 复杂度 | 涉及文件 |
|------|------|--------|----------|
| 1 | 查询结果导出（`\export`） | 中 | `output/export.go`（新建）, `tui/model.go` |
| 2 | `\watch` 实时监控 | 中 | `tui/model.go` |
| 3 | SQL 别名系统 | 中 | `config/config.go`, `tui/model.go` |
| 4 | 会话管理（`\session`） | 高 | `config/config.go`, `tui/model.go` |
| 5 | 智能 SQL 提示 | 低-中 | `completer/snippets.go`（新建）, `completer/completer.go`, `tui/model.go` |
