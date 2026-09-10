# AGENTS.md

This file provides guidance to coding agents (Claude Code, DeepSeek Harness, etc.) when working with code in this repository.

## 项目概述

NothingBot_v4 是一个 Go 编写的 QQ 机器人, 基于 OneBot 11 协议, 通过 EasyOnebot 对接 **NapCat** (`NapCat.Onebot`) 每个功能是一个独立文件里的「模块」 (Module) , 在 `init()` 中注册, 由 `main.go` 统一编排生命周期

**本仓库已纳入 git 版本控制**, 改动/删除文件前先 `git status` / `git diff` 确认工作区, 不要覆盖别人的修改

## 常用命令

```bash
# 构建 (约 35s, 产物名与目录中的预编译二进制一致) 
go build -o NothingBot_v4 .

# 开发运行: env.NoBuild=true, 打印模块初始化顺序与可点击文件链接, 并开启 pprof
go run .
# pprof: http://localhost:6060/debug/pprof/

# 运行单个测试 (测试文件都在根目录 package main, 用 -run 精确指定) 
go test -run TestPixivParseRegexp -v .
go test -run TestEHGalleryUrlReg -v .

# 全部测试 / 静态检查
go test ./...
go vet ./...
```

测试注意事项: 

- 根目录的 `T_*_test.go` 与源码同属 `package main`, `init()` 会照常执行 (`environment.Testing` 为 true 时读内置默认配置、大部分模块被 `Disable`) 
- 涉及 `InitBot().Run()` 的测试需要本机 Redis (`127.0.0.1:6379`) 与 onebot 服务 (`127.0.0.1:8081`) 真实在跑; 纯正则/逻辑测试不需要
- `_test/` 是独立的 `package test` 沙盒 (`go test ./_test/`) , 与机器人本体无关

## 文件命名约定

| 前缀/后缀 | 含义 |
|---|---|
| `M_*.go` | 一个功能模块, 注册消息 matcher |
| `MO_*.go` | 只注册 onebot 事件/通知回调 (如入群退群、戳一戳) , 没有 `Module` 结构体 |
| `M_*_utils.go` | 一族模块共用的配置与辅助函数 (如 `M_MangaParse_utils.go`)  |
| `utils*.go` | 跨模块通用辅助 |
| `T_*_test.go` | 测试 |

子包: `environment/` (运行模式探测) 、`utils/` (泛型工具) 、`slicesyntax/` (页码切片语法 `[1:3][-1]`) 、`qrcode/`、`ocrspace/`、`RSSHub/<name>/` (RSS 源解析器, 实现 `RssHubResource` 接口) 

## 模块生命周期 (核心架构) 

`module.go` 定义 `Module`, 每个模块文件长这样: 

```go
var moduleFoo = Module{
    ModuleMeta: ModuleMeta{
        Name:       fooMId,                  // 即配置键: [modules.<小写名称>]
        Desc:       "描述",                  // /help 里展示
        Conditions: Conditions{REMARK_TOME}, // 帮助信息里的响应条件
        HelpMsg:    "/foo",
        Hidden:     false,                   // true 则不出现在 /help
        SubModules: []*ModuleMeta{...},      // 仅用于帮助信息分组
    },
    Priority: 0,              // 初始化顺序, 越小越先
    Disable:  env.Testing,    // 单元测试下禁用
    Init:      initFoo,       // 必填: 注册 matcher / 读配置
    AfterInit: ...,           // 连接成功拿到 uin 之后 (需要 SelfId 的场景) 
    ReInit:    ...,           // 配置热重载时重新读取
    AtExit:    ...,           // 退出时清理 (停 goroutine、关 http server) 
}

func init() {
    NoBuildPrintFile("M_Foo.go") // 约定: 每个 init 第一行
    modules.Add(&moduleFoo)
}
```

`main.go` 的执行顺序: `init()` 里注册 `moduleMain` -> `Init()` (`modules.Sort()` 按 `Priority` 升序、同优先级按名称排序 -> `InitConfig` -> `Unmarshal` -> `InitLog` -> `InitRedis` -> 各模块 `Init()`) -> 连接 onebot (此时 `SetMsgProcessEnabled(false)`, 消息处理暂停) -> `AfterInit()` -> 开启消息处理

- **`RWMutex` 是模块级并发保护**: 消息处理函数必须用 `moduleX.RWMuWrap(fn)` (有返回值用 `RWMuWrapRet`) 包裹, 它取读锁; `ReInit`/`AtExit` 取写锁, 先 `TryLock` 失败则打印 "still busy" 并阻塞等待新增 matcher 时不要漏掉包裹, 否则重载配置/退出时可能数据竞争
- **`Priority`**: 共享配置加载器用 0 (如 `manga_init`) , 依赖它的模块用 1 (各漫画站点模块) 
- `Disable: env.Testing` 让单元测试不必连接真实 bot

## 消息处理

matcher 在模块 `Init()` 中注册: 

```go
onebot.AddMatcher(moduleFoo.Name.String(), EasyOnebot.NewMatcher().
    OnTypeL1(event.TYPE_L1_MESSAGE).
    IsToMe().                                  // 私聊 / at / bot 别名 (config.onebot.nicknames) 
    IsSuperuser().
    OnRegexpFindAllStringSubmatch(fooReg).
    Do(moduleFoo.RWMuWrap(ctxFoo)))
```

- 正则捕获用 `ctx.Submatches.Get(moduleFoo.Name.String())[0][i]` 取, matcher 名即模块名
- 发送: `ctx.SendMsg/SendMsgf`、`ctx.SendMsgReply/SendMsgReplyf`、`ctx.SendForwardMsgAuto` (合并转发, 自动分批) 、`ctx.UploadGroupFile/UploadPrivateFile`
- 底层 API 在 EasyOnebot 里分三个命名空间: `onebot.Call().Std` (OneBot 标准) 、`.Lgr` (Lagrange 扩展, 本项目实际由 NapCat 兼容实现, 如 `SendGroupForwardMsg`/`GroupPoke`) 、`.Nc` (NapCat 扩展) 写新调用前先确认 NapCat 是否支持该端点
- **实现差异要用 `GetVersionInfoCache().AppName` 分发**: 合并转发消息的节点获取方式在两个实现下完全不同 —— NapCat 直接把 `content` 作为 event 数组返回, Lagrange 需要拿 `id` 再调 `GetForwardMsg`参考 `M_ForwardSlice.go:95` 的 `forwardGetNodes`
- 合并转发: `message.SegmentArray` + `message.Node3(uin, name, content)`
- `utilsCtx.go` 的 `ctxGetMsg/ctxGetType/ctxGetImgSegs/ctxGetUrls` 统一处理「附带媒体」与「回复消息」两种情况
- `PushMsg` / `PushForwardMsg` (utilsBot.go) 用于主动推送, 带重试

## 配置

viper + TOML, `defaultConfig.toml` 通过 `go:embed` 内嵌

- `env.Testing || env.Debugging` 时直接用内嵌默认配置; 否则读 `<二进制目录>/config.toml` (`go run` 时读 `<cwd>/config.toml`) , 文件不存在则写出默认配置并 `os.Exit(0)` 提示编辑
- 全局段 `[log] [global] [onebot]` 解到 `Config` 结构体; `[global].proxy` 会 `os.Setenv` 到 `HTTP_PROXY`/`HTTPS_PROXY`
- 模块配置在 `[modules.<模块名小写>]`, 在模块的 `Init`/`ReInit` 中通过 `config.DecodeModule(moduleId, &cfg)` 读取 (缺失返回 `ErrModuleConfigNotFound`) 可重复的配置用 `[[modules.xxx]]` (crontab、corpus、webhook、bilirelation 等) 
- 配置结构体嵌入 `List` 即获得 `White(ctx)`/`Black(ctx)` 白名单判断 (对应 `whitelist.groups`/`whitelist.users`) 
- **热重载**: fsnotify 监听文件变化 -> `configLoop` -> 逐模块 `ReInit()`; 超级用户私聊 `/reload config` 触发同样流程, `/autoreload toggle` 开关因此模块状态必须在 `ReInit` 里可重建 (停掉旧 goroutine / cron / 重新注册 matcher) 

## 运行模式探测 (environment/) 

由 `os.Executable()` 路径推断: `NoBuild` (`go run`) 、`Testing` (`.test`) 、`Debugging` (`__debug`) `NoBuildPrintFile()` 与 `modules.Sort()` 的打印只在开发时出现, 用于观察模块加载顺序`env.WorkDir` = 当前工作目录, `env.XDir` = 二进制目录; 缓存目录按 `env.WorkDir` 拼接

## 其他基础设施

- **Redis**: `utilsRedis.go` 是对 rueidis 的薄封装 (`RedisClient`、`RedisResult`/`RedisResultList`、`UnmarshalResult[T]`) ; EasyOnebot 自身也用 Redis 存消息缓存 (at/撤回记录查询依赖它) 
- **限流/互斥**: `utilsLimiter.go` 信号量 `Limiter`; `M_MangaParse_utils.go` 的泛型 `Locker[T]` 防止同一 id 被并发解析
- **图片处理**: `salt(data, n)` 在图片末尾追加随机字节, 规避 QQ 服务端图片哈希去重; `imgToJpg`、`NewImagePdf` (fpdf 生成 PDF 后调 `qpdf --linearize` 线性化, 需要系统装 qpdf) 
- **缓存目录**: `BotPixivCache`、`BotEHentaiCache`、`BotNHentaiCache`、`BotJmComicCache`、`BotPicaComicCache`、`BotPermanentLink` 位于仓库根目录, 由模块写入、由外部 nginx 对外提供直链 (如 `pixiv.miuzarte.top/<id>.pdf`) 这些目录内容很大, 不要当作源码改动
- **后台常驻任务** (bili 推送/直播、crontab、RssHub) 在 `AfterInit` 启动、`AtExit` 停止、`ReInit` 重启

## 依赖

`go.mod` 里绝大多数 `github.com/Miuzarte/*` 依赖都通过 `replace` 指向本地检出 `/home/miuzarte/git/<name>` (EasyOnebot、EHentai-go、JMComic-go、biligo、pixiv 等) 改这些库是日常工作流的一部分, 改动立即影响本项目的构建; 反之调试本项目问题时, 也可以直接去那些目录看实现

## 文档与注释风格

- 注释中英文皆可, CJK 与拉丁字母/数字之间留一个空格 (Pangu), 不使用全角标点, 用半角 `, . ( ) / :` 等. 英文注释以句号结尾, 中文注释不加句号
- 不用 `;` 把本该分行的语句挤到一行
- 引用符号时先 `import` 再用短名, 不要写长全限定路径. 本仓库已有 `eh` / `jm` / `nh` / `pc` / `a2d` / `sn` / `stb` 等别名, 沿用即可
- 多余 import 不必手动清理, 交给 gofmt / goimports
- **仓库由人类开发者并行修改**: 改动前先 `git status` / `git diff` 确认工作区, 避免整文件重写、避免破坏既有排版; 宁可多改几次, 也不要用一次性大改动覆盖别人的修改
- 日志统一走全局 `log` (SimpleLog, 等级由 `[log].level` 控制); 面向用户的消息文案直接写在代码里 (本仓库没有 Android 式的资源字符串文件)
- 存量代码的注释大量使用全角标点 (`：` 124 处、`，` 66 处、`。` 7 处等), 上述半角规则**只对新增/修改的注释生效**, 不回头改存量文件
