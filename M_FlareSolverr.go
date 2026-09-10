package main

import (
	"context"
	"regexp"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	fs "github.com/Miuzarte/FlareSolverr-go"
)

const FLARESOLVERR_SCREENSHOT_REGEXP = `(?i)(?:网页截图)\s*(https?://[^\s]+)`

var flareSolverScreenshotReg = regexp.MustCompile(FLARESOLVERR_SCREENSHOT_REGEXP)

type FlareSolverrConfig struct {
	Endpoint string
}

const flareSolverrMId ModuleId = "FlareSolverr"

var (
	flareSolverrConfig = FlareSolverrConfig{}
	// 其他模块初始化时会用到, 提前初始化且不要修改指针本身
	flareSolverrClient = fs.NewClient("http://127.0.0.1:8191/v1")
)

var moduleFlareSolverr = Module{
	ModuleMeta: ModuleMeta{
		Name:    flareSolverrMId,
		Desc:    "FlareSolverr网页截图",
		HelpMsg: "网页截图 <url>",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_FlareSolverr.go")

	moduleFlareSolverr.Init = initFlareSolverr
	moduleFlareSolverr.ReInit = initFlareSolverr
	modules.Add(&moduleFlareSolverr)
}

func initFlareSolverr() {
	err := config.DecodeModule(flareSolverrMId, &flareSolverrConfig)
	if err != nil {
		log.Error(err)
		return
	}

	fsClient := fs.NewClient(flareSolverrConfig.Endpoint)
	*flareSolverrClient = *fsClient

	onebot.AddMatcher(moduleFlareSolverr.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnRegexpFindAllStringSubmatch(flareSolverScreenshotReg).
		Do(moduleFlareSolverr.RWMuWrap(ctxFlareSolverr)),
	)
}

func ctxFlareSolverr(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatch := ctx.Submatches.Get(moduleFlareSolverr.Name.String())[0]
	url := submatch[1]

	resp, err := flareSolverrClient.Get(context.Background(), url, map[string]any{
		fs.PARAM_RETURN_SCREENSHOT: true,
		fs.PARAM_WAIT_IN_SECONDS:   6,
		fs.PARAM_MAX_TIMEOUT:       60000,
	})
	if err != nil {
		ctx.SendMsgReplyf("网页截图失败：%v", err)
		return
	}

	ctx.SendForwardMsgAuto(message.SegmentArray{
		message.Node3(uid, name, message.Text("网页截图："+url)),
		message.Node3(uid, name, message.Image("base64://"+resp.Solution.Screenshot)),
	})
}
