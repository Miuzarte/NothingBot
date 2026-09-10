package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"NothingBot_v4/slicesyntax"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/go-viper/mapstructure/v2"
)

// 本文件的日志 scope
var logForwardSlice = logger.New("ForwardSlice")

// [0]: [SliceSyntaxes]
var forwardSliceReg = regexp.MustCompile(`\[(?:-?\d*:?)*\](?:\[(?:-?\d*:?)*\])*`)

const forwardSliceMId = ModuleId("ForwardSlice")

var moduleForwardSlice = Module{
	ModuleMeta: ModuleMeta{
		Name:       forwardSliceMId,
		Desc:       "对合并转发消息进行切片操作，支持负索引 (Python like)",
		Conditions: Conditions{REMARK_REPLY},
		HelpMsg:    "[start:end] / [index]",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_ForwardSlice.go")

	moduleForwardSlice.Init = initForwardSlice
	modules.Add(&moduleForwardSlice)
}

func initForwardSlice() {
	onebot.AddMatcher(moduleForwardSlice.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnReply().
		OnRegexpFindAllStringSubmatch(forwardSliceReg).
		Do(moduleForwardSlice.RWMuWrap(ctxForwardSlice)),
	)
}

func ctxForwardSlice(ctx *EasyOnebot.Ctx) {
	defer func() {
		if err := recover(); err != nil {
			ctx.SendMsgReply(err)
		}
	}()

	// 获取回复的合并转发消息
	replyMsg, err := ctx.GetReplyMsg()
	if err != nil {
		ctx.SendMsgReplyf("获取消息失败：%v", err)
		return
	}
	forward := replyMsg.Message.GetFirstType(message.TYPE_FORWARD)
	if forward == nil {
		ctx.SendMsgReply("无法操作非合并转发消息")
		return
	}
	forwardNodes, err := forwardGetNodes(*forward)
	if err != nil {
		ctx.SendMsgReplyf("获取合并转发消息失败：%v", err)
		return
	}

	submatch := ctx.Submatches.Get(moduleForwardSlice.Name.String())[0]
	sss := slicesyntax.ParseMulti(submatch[0])
	reply := slicesyntax.DoIndexes(forwardNodes, sss.ToIndexes(len(forwardNodes)))

	// 将 https 转为 http, 避免 trust asia 证书问题
	httpsToHttp(reply)

	_, err = ctx.SendForwardMsgAuto(reply)
	if err != nil {
		ctx.SendMsgReply("切片发送失败")
	}
}

const (
	LAGRANGE_APPNAME = "Lagrange.OneBot"
	NAPCAT_APPNAME   = "NapCat.Onebot"
)

var errNotForward = errors.New("not a forward segment")

func forwardGetNodes(forward message.Segment) (message.SegmentArray, error) {
	if forward.Type != message.TYPE_FORWARD {
		return nil, wrapErr(errNotForward, forward.Type)
	}

	vi, err := onebot.GetVersionInfoCache()
	if err != nil {
		return nil, wrapErr(err, nil)
	}
	switch vi.AppName {
	case NAPCAT_APPNAME:
		return forwardGetNodes_nc(forward)
	case LAGRANGE_APPNAME:
		return forwardGetNodes_lgr(forward)
	default:
		return nil, fmt.Errorf("unsupported onebot implementation: %s", vi.AppName)
	}
}

func forwardGetNodes_nc(forward message.Segment) (message.SegmentArray, error) {
	// napcat 直接以 event 数组的形式返回
	msgEventArr := []*event.Event{}
	err := mapstructure.Decode(forward.Data["content"], &msgEventArr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode forward content: %w", err)
	}
	return eventsToSegArr(msgEventArr), nil
}

func forwardGetNodes_lgr(forward message.Segment) (message.SegmentArray, error) {
	// lgr 需要通过 forward id 再次调用 GetForwardMsg
	forwardId, ok := forward.Data["id"].(string)
	if !ok {
		return nil, fmt.Errorf("fail to get forward id from segment")
	}
	forwardMsg, err := onebot.Call().Std.GetForwardMsg(forwardId)
	if err != nil {
		return nil, fmt.Errorf("fail to get forward msg from id %s: %w", forwardId, err)
	}
	return forwardMsg.Message, nil
}

func eventsToSegArr(events []*event.Event) message.SegmentArray {
	segArr := message.SegmentArray{}
	for _, e := range events {
		name := e.Sender.Card
		if name == "" {
			name = e.Sender.Nickname
		}
		segArr.Append(message.Node3Any(e.Sender.UserId, name, e.Message))
	}
	return segArr
}

var httpReplacer = strings.NewReplacer("https://", "http://")

func httpsToHttp(msg message.SegmentArray) {
	for _, seg := range msg {
		// image, video, record ...
		file, ok := seg.Data["file"].(string)
		if ok {
			seg.Data["file"] = httpReplacer.Replace(file)
		}
		url, ok := seg.Data["url"].(string)
		if ok {
			seg.Data["url"] = httpReplacer.Replace(url)
		}

		// forward nodes:
		contents, ok := seg.Data["content"].([]any)
		if ok {
			for i := range contents {
				contentJ, ok := contents[i].(map[string]any)
				if !ok {
					logForwardSlice.Warn().
						Int("index", i).
						Msg("node content is not map[string]any")
					continue
				}
				data, ok := contentJ["data"].(map[string]any)
				if !ok {
					logForwardSlice.Warn().
						Int("index", i).
						Msg("node content data is not map[string]any")
					continue
				}
				file, ok := data["file"].(string)
				if ok {
					data["file"] = httpReplacer.Replace(file)
				}
				url, ok := data["url"].(string)
				if ok {
					data["url"] = httpReplacer.Replace(url)
				}
			}
		}
	}
}
