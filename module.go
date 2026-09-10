package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Miuzarte/EasyOnebot"
)

type Condition int

const (
	REMARK_TOME       = 1 << iota // 对我说
	REMARK_REPLY                  // 回复消息
	REMARK_WITH_IMAGE             // 附带图片
	REMARK_WITH_MEDIA             // 附带媒体
	REMARK_WITH_FILE              // 附带文件
	REMARK_WHITE_LIST             // 白名单
	REMAKR_ONLY_ADMIN             // 仅管理员
)

func (c Condition) String() string {
	return c.string(1) // 格式化单个 Condition
}

func (c Condition) string(length int) string {
	if c == 0 {
		return ""
	}

	parts := make([]string, 0, 3)

	if c&REMARK_TOME != 0 {
		parts = append(parts, "对我说")
	}
	if c&REMARK_REPLY != 0 {
		parts = append(parts, "回复消息")
	}
	if c&REMARK_WITH_IMAGE != 0 {
		parts = append(parts, "附带图片")
	}
	if c&REMARK_WITH_MEDIA != 0 {
		parts = append(parts, "附带媒体")
	}
	if c&REMARK_WITH_FILE != 0 {
		parts = append(parts, "附带文件")
	}
	if c&REMARK_WHITE_LIST != 0 {
		parts = append(parts, "白名单")
	}
	if c&REMAKR_ONLY_ADMIN != 0 {
		parts = append(parts, "仅管理员")
	}

	if length == 1 {
		return strings.Join(parts, " & ")
	} else {
		return "(" + strings.Join(parts, "&") + ")"
	}
}

type Conditions []Condition

func (cs Conditions) String() string {
	l := len(cs)
	switch l {
	case 0:
		return ""
	case 1:
		return cs[0].string(1)
	}

	items := make([]string, 0, l)
	for _, r := range cs {
		items = append(items, r.string(l))
	}
	return strings.Join(items, " | ")
}

type ModuleMeta struct {
	Name          ModuleId
	Desc          string
	Conditions    Conditions // 响应条件
	HelpMsg       string
	SearchContent string // [TODO] only for search
	Hidden        bool   // 不出现在帮助信息中
}

// Module 模块
type Module struct {
	ModuleMeta

	Priority int // 初始化顺序, 越小越先初始化
	Disable  bool

	Init      func()       // 运行前初始化
	AfterInit func()       // 获取到 uin 后初始化
	RWMutex   sync.RWMutex // 重载配置/退出时读写互斥锁
	ReInit    func()       // 重载配置文件后初始化
	AtExit    func()       // 退出时执行

	SubModules []*ModuleMeta // 子模块, 仅描述信息
}

func (m *Module) RWMuWrap(f func(ctx *EasyOnebot.Ctx)) func(ctx *EasyOnebot.Ctx) {
	return func(ctx *EasyOnebot.Ctx) {
		m.RWMutex.RLock()
		defer m.RWMutex.RUnlock()
		f(ctx)
	}
}

func (m *Module) RWMuWrapRet(f func(ctx *EasyOnebot.Ctx) any) func(ctx *EasyOnebot.Ctx) any {
	return func(ctx *EasyOnebot.Ctx) any {
		m.RWMutex.RLock()
		defer m.RWMutex.RUnlock()
		return f(ctx)
	}
}

type Modules struct {
	M          map[ModuleId]*Module
	SortedKeys []ModuleId
	IsRunning  atomic.Bool
}

func (ms *Modules) Add(m *Module) {
	if ms.IsRunning.Load() {
		log.Panicf("trying to add module (%s) at runtime", m.Name)
	}
	if m.Name == "" {
		log.Panic("empty module name")
	}
	if _, ok := ms.M[m.Name]; ok {
		log.Panicf("duplicate module name: %s", m.Name)
	}
	ms.M[m.Name] = m
}

func (ms *Modules) Sort() {
	modulesTmp := make([]*Module, 0, len(ms.M))
	for k, v := range ms.M {
		if k != v.Name {
			log.Panicf("k(%s) != v.Name(%s)", k, v.Name)
		}
		modulesTmp = append(modulesTmp, v)
	}

	// 按优先级与 ID 排序
	slices.SortFunc(modulesTmp, func(a, b *Module) int {
		return cmp.Or(
			cmp.Compare(a.Priority, b.Priority),
			cmp.Compare(a.Name, b.Name),
		)
	})

	NoBuildPrintln("Modules.SortedKeys:")
	ms.SortedKeys = make([]ModuleId, 0, len(modulesTmp))
	for i, m := range modulesTmp {
		ms.SortedKeys = append(ms.SortedKeys, m.Name)
		NoBuildPrintln(fmt.Sprintf("[%02d](%d) %s", i, m.Priority, m.Name))
	}
}

var modules = Modules{
	M: make(map[ModuleId]*Module),
}
