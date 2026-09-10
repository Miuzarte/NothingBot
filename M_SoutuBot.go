package main

import (
	"context"
	"regexp"
	"strings"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	stb "github.com/Miuzarte/SoutuBot-go"
)

const SOUTU_BOT_RESULT_URL_REGEXP = `soutubot\.moe/results/([0-9A-Za-z_-]+)`

var soutuBotResultUrlReg = regexp.MustCompile(SOUTU_BOT_RESULT_URL_REGEXP)

const soutuBotMId ModuleId = "SoutuBot"

var soutuBotClient *stb.Client

var (
	moduleSoutuBotSearch = ModuleMeta{
		Name:       soutuBotMId.WithSuffix("Search"),
		Desc:       "SoutuBot搜本 (NHentai/EHentai)",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_IMAGE, REMARK_WITH_IMAGE},
		HelpMsg:    "\"搜本\" / \"/soutubot\"",
	}
	moduleSoutuBotResult = ModuleMeta{
		Name:       soutuBotMId.WithSuffix("Result"),
		Desc:       "SoutuBot搜本 (NHentai/EHentai)",
		Conditions: Conditions{},
		HelpMsg:    SOUTU_BOT_RESULT_URL_REGEXP,
	}
)

var moduleSoutuBot = Module{
	ModuleMeta: ModuleMeta{
		Name:   soutuBotMId,
		Hidden: true,
	},
	Priority: 1, // after [moduleFlareSolverr]
	Disable:  env.Testing,
	SubModules: []*ModuleMeta{
		&moduleSoutuBotSearch,
		&moduleSoutuBotResult,
	},
}

func init() {
	NoBuildPrintFile("M_SoutuBot.go")

	moduleSoutuBot.Init = initSoutuBot
	moduleSoutuBot.ReInit = initSoutuBot
	modules.Add(&moduleSoutuBot)
}

func initSoutuBot() {
	soutuBotClient = stb.NewClient(flareSolverrClient)

	onebot.AddMatcher(moduleSoutuBotSearch.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnStringsContains("搜本", "/soutubot").
		OnFunc(func(c *EasyOnebot.Ctx) bool {
			return !strings.Contains(c.UnescapedMessage, "//soutubot.moe")
		}).
		Do(moduleSoutuBot.RWMuWrap(ctxSoutuBotSearch)),
	)
	onebot.AddMatcher(moduleSoutuBotResult.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnRegexpFindAllStringSubmatch(soutuBotResultUrlReg).
		Do(moduleSoutuBot.RWMuWrap(ctxSoutuBotResult)),
	)
}

func ctxSoutuBotSearch(ctx *EasyOnebot.Ctx) {
	imgSegs, err := ctxGetImgSegs(ctx)
	if err != nil {
		if ctx.IsToMe {
			ctx.SendMsgReplyf("[SoutuBot] %s", err.Error())
		}
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()
	rs := NewReverseSearch(ctx, tctx, REVERSE_SEARCH_SITE_STB, imgSegs[0], true)
	forward, err, raw := rs.Do()
	if err != nil {
		ctx.SendMsgf("[SoutuBot] 搜索失败：%v", err)
		return
	}

	_, err = ctx.SendForwardMsgAuto(forward)
	if err != nil {
		ctx.SendMsg("[SoutuBot] 结果合并转发发送失败")
		if raw, ok := raw.(*stb.Response); ok {
			ctx.SendMsgf("%s", raw.ResultUrl())
		}
		return
	}
}

func ctxSoutuBotResult(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	nickname := ctx.Event.Sender.GetCardOrNickname()
	submatch := ctx.Submatches.Get(moduleSoutuBotResult.Name.String())[0]

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()
	resp, err := soutuBotClient.GetResult(tctx, submatch[1])
	if err != nil {
		if err, ok := err.(*stb.HttpError); ok {
			onebot.Log2Sus.Errorf("%v", *err)
		}
		ctx.SendMsgf("[SoutuBot] 结果获取失败：%v", err)
		return
	}

	resultsSegChains := soutubotBuildNodes(resp)
	forward := make(message.SegmentArray, 0, len(resultsSegChains))
	for _, segChain := range resultsSegChains {
		forward.Append(message.Node3(uid, nickname, segChain))
	}

	_, err = ctx.SendForwardMsgAuto(forward)
	if err != nil {
		ctx.SendMsg("[SoutuBot] 结果合并转发发送失败")
		return
	}
}
