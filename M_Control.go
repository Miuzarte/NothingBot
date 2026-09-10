package main

import (
	"regexp"
	"strings"

	"NothingBot_v4/logger"
	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

// 本文件的日志 scope
var logControl = logger.New("Control")

var regAdminRepeat = regexp.MustCompile(`(?si)(?:--\S+\s*)*(\S+)\s*([\s\S]+)`) // (.+) 不能匹配换行符

const controlMId ModuleId = "Control"

var moduleControl = Module{
	ModuleMeta: ModuleMeta{
		Name:   controlMId,
		Hidden: true,
	},
}

func init() {
	NoBuildPrintFile("M_Control.go")

	moduleControl.Init = initControl
	modules.Add(&moduleControl)
}

func initControl() {
	onebot.AddMatcher(moduleControl.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsSuperuser().
		IsToMe().
		OnRegexpFindAllStringSubmatch(regAdminRepeat).
		Do(moduleControl.RWMuWrap(ctxControl)),
	)
}

func ctxControl(ctx *EasyOnebot.Ctx) {
	var msg any
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pReply := strings.Contains(lowerRm, "--reply")
	match := ctx.Submatches.Get(moduleControl.Name.String())[0]
	switch match[1] {
	case "repeat", "复读":
		msg = match[2]
	case "message.Text":
		msg = message.Text(match[2])
	case "message.Image":
		msg = message.Image(match[2])
	default:
		return
	}
	var err error
	if !pReply {
		_, err = ctx.SendMsg(msg)
	} else {
		_, err = ctx.SendMsgReply(msg)
	}
	if err != nil {
		logControl.Warn().
			Err(err).
			Msg("failed to SendMsg")
	}
}
