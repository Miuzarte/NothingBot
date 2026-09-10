package main

import (
	"context"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"

	sn "github.com/Miuzarte/SauceNAO-go"
)

type SauceNaoConfig struct {
	ApiKey       string
	OverrideHost string
	Hide         int

	LowSimThreshold float64
	HideWhenLowSim  bool
}

const sauceNaoMId ModuleId = "SauceNao"

var (
	sauceNaoConfig = SauceNaoConfig{}
	sauceNaoClient *sn.Client
)

var moduleSauceNao = Module{
	ModuleMeta: ModuleMeta{
		Name:       sauceNaoMId,
		Desc:       "SauceNAO搜图",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_IMAGE, REMARK_WITH_IMAGE},
		HelpMsg:    "\"搜图\" / \"/saucenao\"",
	},
	Priority: 1, // after [moduleFlareSolverr]
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_SauceNao.go")

	moduleSauceNao.Init = initSauceNao
	moduleSauceNao.ReInit = initSauceNao
	modules.Add(&moduleSauceNao)
}

func initSauceNao() {
	err := config.DecodeModule(sauceNaoMId, &sauceNaoConfig)
	if err != nil {
		log.Error(err)
		return
	}

	sauceNaoClient = sn.NewClient(
		sauceNaoConfig.ApiKey,
		sauceNaoConfig.OverrideHost,
		2,
		sauceNaoConfig.Hide,
		flareSolverrClient,
	)

	onebot.AddMatcher(moduleSauceNao.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnStringsContains("搜图", "/saucenao").
		Do(moduleSauceNao.RWMuWrap(ctxSauceNao)),
	)
}

func ctxSauceNao(ctx *EasyOnebot.Ctx) {
	imgSegs, err := ctxGetImgSegs(ctx)
	if err != nil {
		if ctx.IsToMe {
			ctx.SendMsgReplyf("[SauceNAO] %s", err.Error())
		}
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()
	rs := NewReverseSearch(ctx, tctx, REVERSE_SEARCH_SITE_SN, imgSegs[0], false)
	forward, err, _ := rs.Do()
	if err != nil {
		ctx.SendMsgf("[SauceNAO] 搜索失败：%v", err)
		return
	}

	_, err = ctx.SendForwardMsgAuto(forward)
	if err != nil {
		ctx.SendMsgf("[SauceNAO] 结果合并转发发送失败：%v", err)
		return
	}
}
