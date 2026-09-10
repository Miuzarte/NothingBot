package main

import (
	"context"
	"encoding/json"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"

	a2d "github.com/Miuzarte/Ascii2d-go"
)

type Ascii2dConfig struct {
	OverrideHost string
}

const ascii2dMId ModuleId = "Ascii2d"

var (
	ascii2dConfig = Ascii2dConfig{}
	ascii2dClient *a2d.Client
)

var moduleAscii2d = Module{
	ModuleMeta: ModuleMeta{
		Name:       ascii2dMId,
		Desc:       "Ascii2d搜图",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_IMAGE, REMARK_WITH_IMAGE},
		HelpMsg:    "\"搜图\" / \"/ascii2d\"",
	},
	Priority: 1, // after [moduleFlareSolverr]
	Disable:  env.Testing,
}

func init() {
	NoBuildPrintFile("M_Ascii2d.go")

	moduleAscii2d.Init = initAscii2d
	moduleAscii2d.ReInit = initAscii2d
	modules.Add(&moduleAscii2d)
}

func initAscii2d() {
	err := config.DecodeModule(ascii2dMId, &ascii2dConfig)
	if err != nil {
		log.Error(err)
		return
	}

	ascii2dClient = a2d.NewClient(
		ascii2dConfig.OverrideHost,
		flareSolverrClient,
	)

	onebot.AddMatcher(moduleAscii2d.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnStringsContains("搜图", "/ascii2d").
		Do(moduleAscii2d.RWMuWrap(ctxAscii2d)),
	)
}

func ctxAscii2d(ctx *EasyOnebot.Ctx) {
	imgSegs, err := ctxGetImgSegs(ctx)
	if err != nil {
		if ctx.IsToMe {
			ctx.SendMsgReplyf("[Ascii2d] %s", err.Error())
		}
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()
	rs := NewReverseSearch(ctx, tctx, REVERSE_SEARCH_SITE_A2D, imgSegs[0], false)
	forward, err, _ := rs.Do()
	if err != nil {
		ctx.SendMsgf("[Ascii2d] 搜索失败：%v，可能是图片过大(>10MB)，可尝试重新截图再次搜图", err)
		return
	}

	_, err = ctx.SendForwardMsgAuto(forward)
	if err != nil {
		j, _ := json.Marshal(forward)
		ctx.SendMsgf("[Ascii2d] 结果合并转发发送失败：%v\n\n%s", err, j)
		return
	}
}
