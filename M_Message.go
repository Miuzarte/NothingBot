package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/utils"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

const messageMId ModuleId = "Message"

var (
	moduleMessageUrl = ModuleMeta{
		Name:       messageMId.WithSuffix("Url"),
		Desc:       "获取图片/视频/语音/文件消息的直链",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_FILE, REMARK_WITH_IMAGE},
		HelpMsg:    "--url",
	}
	moduleMessageLink = ModuleMeta{
		Name:       messageMId.WithSuffix("Link"),
		Desc:       "获取图片/视频/语音/文件消息的永久链接",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_FILE, REMARK_WITH_IMAGE},
		HelpMsg:    "--link",
	}
	moduleMessageToMessage = ModuleMeta{
		Name:       messageMId.WithSuffix("ToMessage"),
		Desc:       "将文件消息重发为图片/视频/语音消息",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_FILE},
		HelpMsg: "--to-message" +
			"\n" + "别名：--tomsg, --to-msg, --tomessage",
	}
)

var moduleMessage = Module{
	ModuleMeta: ModuleMeta{
		Name: messageMId,
		Desc: "一些操作消息的工具",
	},
	Disable: env.Testing,
	SubModules: []*ModuleMeta{
		&moduleMessageUrl,
		&moduleMessageToMessage,
	},
}

func init() {
	NoBuildPrintFile("M_Message.go")

	moduleMessage.Init = initMessage
	modules.Add(&moduleMessage)
}

func initMessage() {
	onebot.AddMatcher(moduleMessageUrl.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnStringsContains("--url").
		Reply(moduleMessage.RWMuWrapRet(ctxMessageUrl)),
	)
	onebot.AddMatcher(moduleMessageLink.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnStringsContains("--link").
		Send(moduleMessage.RWMuWrapRet(ctxMessageLink)),
	)
	onebot.AddMatcher(moduleMessageToMessage.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnStringsContains("--tomsg", "--to-msg", "--tomessage", "--to-message").
		Send(moduleMessage.RWMuWrapRet(ctxAnyToMsg)),
	)
}

func ctxMessageUrl(ctx *EasyOnebot.Ctx) any {
	mediaUrls, _, err := ctxGetUrls(ctx)
	if err != nil {
		return err
	}
	return strings.Join(mediaUrls, "，")
}

func ctxMessageLink(ctx *EasyOnebot.Ctx) any {
	tctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	segs, err := ctxGetType(ctx,
		message.TYPE_IMAGE,
		message.TYPE_RECORD,
		message.TYPE_VIDEO,
		message.TYPE_FILE,
	)
	if err != nil {
		return err
	}

	for _, seg := range segs {
		const folder = "BotPermanentLink"
		name, ok := seg.Data["file"].(string)
		if !ok {
			return "[ERROR] unable to get resource name"
		}
		path := filepath.Join(folder, name)

		var size int
		var url string
		var body []byte

		f, _ := os.Open(path)
		if f != nil {
			fi, err := f.Stat()
			if err == nil {
				size = int(fi.Size())
				goto CLOSE_AND_SEND
			}
		}

		url, _ = seg.GetUrl()
		if url == "" {
			return "[ERROR] unable to get resource url"
		}
		body, err = seg.Download(tctx)
		if err != nil {
			return err
		}

		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o666)
		if err != nil {
			return err
		}

		size, err = f.Write(body)
		if err != nil {
			f.Close()
			// remove partial file on write failure
			_ = os.Remove(path)
			return err
		}

	CLOSE_AND_SEND:

		if err := f.Close(); err != nil {
			// remove file if close fails (prevent serving corrupt file)
			_ = os.Remove(path)
			return err
		}

		ctx.SendMsgReplyf(
			"(%s) qq.miuzarte.top/permalink/%s",
			utils.FormatBytes(uint64(size)), name,
		)
	}

	return nil
}

func ctxAnyToMsg(ctx *EasyOnebot.Ctx) any {
	fileSegs, err := ctxGetType(ctx, message.TYPE_FILE)
	if err != nil {
		return err
	}
	if len(fileSegs) == 0 {
		return fmt.Errorf("need a file message")
	}
	fileSeg := fileSegs[0] // 只会有一个

	fileSize := fileSeg.Data["file_size"].(int)
	if fileSize > 32*1024*1024 {
		return fmt.Errorf("file size too large: %s > 32 MiB", utils.FormatBytes(uint64(fileSize)))
	}

	data, err := fileSeg.Download(context.Background())
	if err != nil {
		return err
	}
	ct := http.DetectContentType(data)
	if len(ct) <= 6 {
		goto RET
	}
	switch ct[:6] {
	case "image/":
		switch ct[6:] {
		case "bmp", "gif", "webp", "png", "jpeg":
			return message.Image(data)
			// case "icon":
		}

	case "video/":
		return message.Video(data)

	case "audio/":
		// switch ct[6:] {}
		return message.Record(data)

	}

RET:
	return fmt.Errorf("unsupported content type: %s", ct)
}
