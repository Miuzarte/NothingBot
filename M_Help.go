package main

import (
	"regexp"
	"strings"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

const ABOUT_INFO = `NothingBot_v4 by Miuzarte` // TODO: complete about info

var helpReg = regexp.MustCompile(`(?i)/help\s*(.*)$`)

const (
	helpMId  ModuleId = "Help"
	noteMId  ModuleId = "Note"
	aboutMId ModuleId = "About"
)

var allHelpMsg message.SegmentArray

var moduleNote = ModuleMeta{
	Name:    noteMId,
	Desc:    "提示",
	HelpMsg: "“对我说”逻辑：在私聊、消息中带有at或bot别名时触发",
}

var moduleAbout = ModuleMeta{
	Name: aboutMId,
	Desc: ABOUT_INFO,
}

var moduleHelp = Module{
	ModuleMeta: ModuleMeta{
		Name:       helpMId,
		Desc:       "显示(指定)模块帮助信息",
		Conditions: Conditions{REMARK_TOME},
		HelpMsg:    "/help\n/help <模块名/关键字>",
	},
	Disable: env.Testing,
	SubModules: []*ModuleMeta{
		&moduleNote,
		&moduleAbout,
	},
}

func init() {
	NoBuildPrintFile("M_Help.go")

	moduleHelp.Init = initHelp
	modules.Add(&moduleHelp)

	// [TODO] fix for new modules management
	// modules.Add(&Module{
	// 	ModuleMeta: moduleNote,
	// 	Disable:    true,
	// })
	// modules.Add(&Module{
	// 	ModuleMeta: moduleAbout,
	// 	Disable:    true,
	// })
}

func initHelp() {
	onebot.AddMatcher(moduleHelp.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsToMe().
		OnRegexpFindAllStringSubmatch(helpReg).
		Do(moduleHelp.RWMuWrap(ctxHelp)),
	)
}

func ctxHelp(ctx *EasyOnebot.Ctx) {
	if len(allHelpMsg) == 0 {
		lazyInitHelp(ctx.Event.SelfId)
	}

	keyword := ctx.Submatches.Get(moduleHelp.Name.String())[0][1]
	if keyword == "" {
		ctx.SendForwardMsgAuto(allHelpMsg)
		return
	}

	modules := searchModules(keyword)
	if l := len(modules); l == 0 {
		ctx.SendMsgf("模块 %s 不存在", keyword)
		return
	} else if l == 1 {
		ctx.SendMsg(formatModuleHelp(modules[0]))
	} else {
		reply := message.SegmentArray{}
		for _, module := range modules {
			reply.Append(
				message.Node3(ctx.Event.SelfId, "", formatModuleHelp(module)),
			)
		}
		ctx.SendForwardMsgAuto(reply)
	}
}

func lazyInitHelp(selfId int) {
	allHelpMsg = make(message.SegmentArray, 0, len(modules.M))
	// [moduleHelp] 在最前
	allHelpMsg.Append(
		message.Node3(selfId, "", formatModuleHelp(&moduleHelp)),
	)
	for _, k := range modules.SortedKeys {
		module := modules.M[k]
		if module == &moduleHelp {
			continue
		}
		segChain := formatModuleHelp(module)
		if len(segChain) == 0 {
			continue
		}
		allHelpMsg.Append(message.Node3(selfId, "", segChain))
	}
}

// searchModules 搜索模块的名称与描述
func searchModules(kw string) (m []*Module) {
	kwLower := strings.ToLower(kw)
	for _, key := range modules.SortedKeys {
		module := modules.M[key]
		if module.Hidden {
			continue
		}
		if strings.Contains(strings.ToLower(module.Name.String()), kwLower) ||
			strings.Contains(strings.ToLower(module.Desc), kwLower) ||
			strings.Contains(strings.ToLower(module.SearchContent), kwLower) {
			m = append(m, modules.M[key])
		}
	}
	return
}

func formatModuleHelp(m *Module) (segChain message.SegmentArray) {
	if !m.Hidden {
		segChain.Append(formatModuleMetaHelp(&m.ModuleMeta)...)
	}
	for i, meta := range m.SubModules {
		if i != 0 || !m.Hidden {
			segChain.Append(message.Text("\n\n\n"))
		}
		segChain.Append(formatModuleMetaHelp(meta)...)
	}
	return
}

func formatModuleMetaHelp(meta *ModuleMeta) (segChain message.SegmentArray) {
	if meta.Desc != "" {
		segChain.Append(message.Textf("%s：%s", meta.Name, meta.Desc))
	} else {
		segChain.Append(message.Text(meta.Name))
	}

	if len(meta.Conditions) != 0 {
		segChain.Append(message.Textf("\n条件：%s", meta.Conditions))
	}

	if meta.HelpMsg != "" {
		segChain.Append(message.Textf("\n\n%s", meta.HelpMsg))
	}

	return
}
