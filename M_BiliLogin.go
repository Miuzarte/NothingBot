package main

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"NothingBot_v4/qrcode"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/Miuzarte/EasyOnebot"

	"github.com/Miuzarte/biligo"
)

const biliLoginMId ModuleId = "BiliLogin"

var moduleBiliLogin = Module{
	ModuleMeta: ModuleMeta{
		Name:       biliLoginMId,
		Desc:       "Bilibili登录",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_BiliLogin.go")

	moduleBiliLogin.Init = initBiliLogin
	moduleBiliLogin.AtExit = saveIdentity
	modules.Add(&moduleBiliLogin)

	loadIdentity() // 保证在推送模块之前加载
}

func initBiliLogin() {
	onebot.AddMatcher(moduleBiliLogin.Name.String(), EasyOnebot.NewMatcher().
		OnTypes([]string{event.TYPE_L1_MESSAGE}, []string{event.TYPE_L2_MESSAGE_PRIVATE}).
		IsSuperuser().
		IsNotCardMsg().
		IsNotForwardMsg().
		OnRegexpMatchString(regexp.MustCompile(`(?i)^/(check cookie|update buvid34|qrcode login)$`)).
		Do(moduleBiliLogin.RWMuWrap(ctxBiliLogin)),
	)
}

var (
	biliLoginCtx    context.Context
	biliLoginCancel context.CancelFunc
)

func ctxBiliLogin(ctx *EasyOnebot.Ctx) {
	switch strings.ToLower(ctx.Event.RawMessage) {
	case "/check cookie":
		b, err := json.Marshal(biligo.ExportIdentity())
		if err != nil {
			ctx.SendMsgf("failed to marshal identity: %v", err)
		} else {
			ctx.SendMsg(b)
		}

	case "/update buvid34":
		err := biligo.CookieUpdateBuvid34()
		if err != nil {
			ctx.SendMsgf("failed to update buvid34: %v", err)
		} else {
			ctx.SendMsg("update buvid34 success")
		}

	case "/qrcode login":
		if biliLoginCancel != nil {
			biliLoginCancel()
			biliLoginCtx = nil
			biliLoginCancel = nil
			ctx.SendMsgf("已取消")
			return
		}

		biliLoginCtx, biliLoginCancel = context.WithCancel(context.Background())
		defer func() {
			biliLoginCancel()
			biliLoginCtx = nil
			biliLoginCancel = nil
		}()

		qrcodeUrl, it, err := biligo.Login(biliLoginCtx)
		if err != nil {
			ctx.SendMsgf("failed to fetch qrcode: %v", err)
			return
		}
		qrc, err := qrcode.New(qrcodeUrl, 512)
		if err != nil {
			ctx.SendMsgf("failed to create qrcode: %v", err)
			return
		}
		_, err = ctx.SendMsg(message.Image(qrc))
		if err != nil {
			log.Error("[BiliLogin] failed to send qrcode: ", err)
			return
		}

		for code, err := range it {
			if err != nil {
				ctx.SendMsgf("failed to poll login status: %v", err)
				return
			}
			switch code {
			case biligo.LOGIN_CODE_STATE_SUCCESS:
				ctx.SendMsgf("登录成功")
				saveIdentity()
				return
			case biligo.LOGIN_CODE_STATE_EXPIRED:
				ctx.SendMsgf("二维码已失效")
				return
			}
		}

	}
}

func loadIdentity() {
	f, err := os.OpenFile("./bilibili_identity", os.O_RDONLY, 0o666)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("bilibili_identity not exist, need login")
		} else {
			log.Error("open file error: ", err)
		}
		return
	}
	id := biligo.Identity{}
	err = json.NewDecoder(f).Decode(&id)
	if err != nil {
		log.Error("decode error: ", err)
		return
	}
	biligo.ImportIdentity(id)
}

func saveIdentity() {
	id := biligo.ExportIdentity()
	f, err := os.OpenFile("./bilibili_identity", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		log.Error("open file error: ", err)
		return
	}
	err = json.NewEncoder(f).Encode(id)
	if err != nil {
		log.Error("encode error: ", err)
		return
	}
}
