package main

import (
	"regexp"
	"strings"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"

	env "NothingBot_v4/environment"
)

var (
	removeCqCode        = regexp.MustCompile(`\[CQ:.*?]\s*`)
	aiReply2077Reg      = regexp.MustCompile(`[吗？?]\s*$|是不是`)
	aiReply2077Replacer = strings.NewReplacer("你", "我", "是不是", "是", "吗", "", "？", "！", "?", "!")
)

const aiReply2077MId ModuleId = "AiReply2077"

var moduleAiReply2077 = Module{
	ModuleMeta: ModuleMeta{
		Name:       aiReply2077MId,
		Desc:       "超未来仿生人工智能对话",
		Conditions: Conditions{REMARK_TOME},
		HelpMsg:    "尝试对我说 \"你是不是高性能机器人？\"",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_AiReply2077.go")

	moduleAiReply2077.Init = initAiReply2077
	modules.Add(&moduleAiReply2077)
}

func initAiReply2077() {
	onebot.AddMatcher(moduleAiReply2077.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnRegexpMatchString(aiReply2077Reg).
		IsToMe().
		Reply(moduleAiReply2077.RWMuWrapRet(ctxAiReply2077)),
	)
}

func ctxAiReply2077(ctx *EasyOnebot.Ctx) any {
	return aiReply2077Replacer.Replace(
		removeCqCode.ReplaceAllString(
			ctx.Event.RawMessage, "",
		),
	)
}
