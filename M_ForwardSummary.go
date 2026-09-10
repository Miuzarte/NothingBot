package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"NothingBot_v4/ocrspace"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/Miuzarte/openai-go/v3"
	"github.com/go-viper/mapstructure/v2"
)

const forwardSummaryMId = ModuleId("ForwardSummary")

var moduleForwardSummary = Module{
	ModuleMeta: ModuleMeta{
		Name:       forwardSummaryMId,
		Desc:       "对合并转发消息进行总结，支持图片与递归",
		Conditions: Conditions{REMARK_REPLY},
		HelpMsg:    "总结一下",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_ForwardSummary.go")

	moduleForwardSummary.Init = initForwardSummary
	modules.Add(&moduleForwardSummary)
}

func initForwardSummary() {
	onebot.AddMatcher(moduleForwardSummary.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnReply().
		OnStringsContains("总结一下").
		Do(moduleForwardSummary.RWMuWrap(ctxForwardSummary)),
	)
}

func ctxForwardSummary(ctx *EasyOnebot.Ctx) {
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pDebug := strings.Contains(lowerRm, "--debug")
	pTest := strings.Contains(lowerRm, "--test")
	if pTest {
		pDebug = true
	}

	// 获取回复的合并转发消息
	replyMsg, err := ctx.GetReplyMsg()
	if err != nil {
		ctx.SendMsgReplyf("获取消息失败：%v", err)
		return
	}
	forward := replyMsg.Message.GetFirstType(message.TYPE_FORWARD)
	if forward == nil {
		ctx.SendMsgReply("无法总结非合并转发消息")
		return
	}

	simplified, err := simplifyForward(*forward, nil)
	if err != nil {
		ctx.SendMsgReplyf("解析合并转发消息失败：%v", err)
		return
	}
	jsonChatPairs, err := json.MarshalIndent(simplified, "", "  ")
	if err != nil {
		ctx.SendMsgReplyf("序列化合并转发消息失败：%v", err)
		return
	}

	const sysPrompt = `你是消息总结助手。请按要点总结我提供的聊天内容：
不改变事实，不臆测；
对于图片，我会尽量提供图片的 OCR 结果；
以陈述格式输出，不需要罗列分点。`
	userMessage := "开始总结以下内容：\n" + string(jsonChatPairs)

	if pDebug {
		ctx.SendMsg(userMessage, true)
		if pTest { // 仅发送内容不请求
			return
		}
	}

	_, resp, err := deepSeekCompletion(DEEPSEEK_V3, []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(sysPrompt),
		openai.UserMessage(userMessage),
	})
	if err != nil {
		ctx.SendMsgf("请求总结失败：%v", err)
		return
	}

	_, err = ctx.SendMsgReply(resp)
	if err != nil {
		ctx.SendMsg("总结消息发送失败")
		return
	}
}

type forwardSummaryNode struct {
	Nickname string `json:"nickname"`
	Type     string `json:"type"`
	Content  any    `json:"content"` // string | []forwardSummaryNode
}

var errUnexpectedForwardType = errors.New("unexpected forward segment type")

func simplifyForward(forward message.Segment, imgContent map[string]string) ([]forwardSummaryNode, error) {
	forwardNodes, err := forwardGetNodes(forward)
	if err != nil {
		return nil, wrapErr(err, forward)
	}
	summaryNodes := []forwardSummaryNode{}
	for _, seg := range forwardNodes {
		if seg.Type != message.TYPE_NODE {
			return nil, wrapErr(errUnexpectedForwardType, seg.Type)
		}

		nickname := seg.Data["nickname"].(string)
		content := message.SegmentArray{}
		err := mapstructure.Decode(seg.Data["content"], &content)
		if err != nil {
			return nil, wrapErr(err, seg.Data["content"])
		}

		if forward := content.GetFirstType(message.TYPE_FORWARD); forward != nil {
			// 递归处理子合并转发
			subNodes, err := simplifyForward(*forward, imgContent)
			if err != nil {
				return nil, wrapErr(err, forward)
			}
			summaryNodes = append(summaryNodes, forwardSummaryNode{
				Nickname: nickname,
				Type:     "sub messages group",
				Content:  subNodes,
			})
		} else {
			// 普通消息
			switch len(content) {
			case 0:
				continue
			case 1:
				seg := content[0]
				segStr := seg.String()
				if seg.Type == message.TYPE_IMAGE {
					// 尝试 OCR 图片；成功则仅追加 OCR 结果，失败则仅追加 image url；非图片则仅追加文本
					imgUrl, ok := seg.Data["url"].(string)
					if ok {
						resp, err := http.Get(imgUrl)
						if err == nil && resp.StatusCode/100 == 2 {
							defer resp.Body.Close()
							ocrResp, err := ocrspace.Do(resp.Body, ocrspaceConfig)
							if err == nil && !ocrResp.IsErroredOnProcessing {
								summaryNodes = append(summaryNodes, forwardSummaryNode{
									Nickname: nickname,
									Type:     "image message ocr result",
									Content:  strings.Join(ocrResp.Texts(), "\n"),
								})
								continue
							}
						}
					}
					summaryNodes = append(summaryNodes, forwardSummaryNode{
						Nickname: nickname,
						Type:     "image url (ocr failed)",
						Content:  segStr,
					})
					continue
				}
				summaryNodes = append(summaryNodes, forwardSummaryNode{
					Nickname: nickname,
					Type:     "text message",
					Content:  segStr,
				})
			default: // 可能由于 emoji 意外分离, 重新合并
				s := make([]string, 0, len(content))
				for _, seg := range content {
					s = append(s, seg.String())
				}
				summaryNodes = append(summaryNodes, forwardSummaryNode{
					Nickname: nickname,
					Type:     "text message",
					Content:  strings.Join(s, ""),
				})
			}
		}
	}
	return summaryNodes, nil
}
