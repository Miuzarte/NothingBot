package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"NothingBot_v4/qrcode"

	qrdecoder "github.com/tuotoo/qrcode"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

const QRCODE_ENCODE_REGEXP = `(?s)(?:制作二维码|制码)[\s:：]*(.*)`

var qrcodeEncodeReg = regexp.MustCompile(QRCODE_ENCODE_REGEXP)

const qrcodeMId ModuleId = "Qrcode"

var (
	moduleQrcodeEncode = ModuleMeta{
		Name:       qrcodeMId.WithSuffix("Encode"),
		Desc:       "为给定内容制作二维码",
		Conditions: Conditions{REMARK_TOME, REMARK_TOME | REMARK_REPLY},
		HelpMsg:    QRCODE_ENCODE_REGEXP,
	}
	moduleQrcodeDecode = ModuleMeta{
		Name:       qrcodeMId.WithSuffix("Decode"),
		Desc:       "扫描二维码",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_IMAGE, REMARK_WITH_IMAGE},
		HelpMsg:    "\"扫码\" / \"/scanqr\" / \"/qrscan\"",
	}
)

var moduleQrcode = Module{
	ModuleMeta: ModuleMeta{
		Name: qrcodeMId,
		Desc: "二维码工具",
	},
	Disable: env.Testing,
	SubModules: []*ModuleMeta{
		&moduleQrcodeEncode,
		&moduleQrcodeDecode,
	},
}

func init() {
	NoBuildPrintFile("M_Qrcode.go")

	moduleQrcode.Init = initQrcode
	modules.Add(&moduleQrcode)
}

func initQrcode() {
	onebot.AddMatcher(moduleQrcodeEncode.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		IsToMe().
		OnRegexpFindAllStringSubmatch(qrcodeEncodeReg).
		Reply(moduleQrcode.RWMuWrapRet(ctxQrcodeEncode)),
	)
	onebot.AddMatcher(moduleQrcodeDecode.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnStringsContains("扫码", "/scanqr", "/qrscan").
		Reply(moduleQrcode.RWMuWrapRet(ctxQrcodeDecode)),
	)
}

func ctxQrcodeEncode(ctx *EasyOnebot.Ctx) any {
	content := ctx.Submatches.Get(moduleQrcodeEncode.Name.String())[0][1]
	if content == "" {
		msg, err := ctxGetMsg(ctx)
		if err != nil {
			return err
		}
		content = msg.String()
	}
	if len(content) > 1024 {
		return fmt.Errorf("content size %d exceeds limit 1024", len(content))
	}
	qrc, err := qrcode.New(content, 512)
	if err != nil {
		return fmt.Errorf("生成二维码失败：%w", err)
	}
	return message.Image(qrc)
}

func ctxQrcodeDecode(ctx *EasyOnebot.Ctx) any {
	imgSegs, err := ctxGetImgSegs(ctx)
	if err != nil {
		if ctx.IsToMe {
			return err
		} else {
			return nil
		}
	}

	results := make([]string, 0, len(imgSegs))
	for _, imgSeg := range imgSegs {
		data, err := imgSeg.Download(context.Background())
		if err != nil {
			return err
		}
		ct := http.DetectContentType(data)
		if !strings.HasPrefix(ct, "image/") {
			return fmt.Errorf("not an image content: %s", ct)
		}
		qrmatrix, err := qrdecoder.Decode(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("failed to decode: %w", err)
		}
		results = append(results, qrmatrix.Content)
	}

	return strings.Join(results, "\n")
}
