package main

import (
	"NothinBot/EasyBot"
	"regexp"

	log "github.com/sirupsen/logrus"
)

func initParse() {
	switch summaryBackend = v.GetString("parse.settings.summaryBackend"); summaryBackend {
	case "glm":
		selectedModelStr = "ChatGLM2-6B"
	case "qianfan":
		switch selectedModelStr = v.GetString("qianfan.model"); selectedModelStr {
		case "ERNIE_Bot":
			selectedModel = qianfanModels.ERNIE_Bot
		case "ERNIE_Bot_turbo":
			selectedModel = qianfanModels.ERNIE_Bot_turbo
		case "BLOOMZ_7B":
			selectedModel = qianfanModels.BLOOMZ_7B
		case "Llama_2_7b":
			selectedModel = qianfanModels.Llama_2_7b
		case "Llama_2_13b":
			selectedModel = qianfanModels.Llama_2_13b
		case "Llama_2_70b":
			selectedModel = qianfanModels.Llama_2_70b
		default:
			log.Warn("[summary] 总结使用的千帆大语言模型配置不正确，使用默认设置: ERNIE_Bot_turbo")
			selectedModel = qianfanModels.ERNIE_Bot_turbo
		}
	default:
		summaryBackend = "glm"
		selectedModelStr = "ChatGLM2-6B"
		log.Warn("[summary] 总结使用的语言大模型后端配置不正确，使用默认设置: ChatGLM")
	}
	if summaryBackend == "baidu" && v.GetString("qianfan.keys.api") == v.GetString("qianfan.keys.secret") {
		log.Warn("[summary] 未配置千帆 api key, 使用默认设置: ChatGLM")
		summaryBackend = "glm"
		selectedModelStr = "ChatGLM2-6B"
	}
	log.Info("[summary] 内容总结使用后端: ", summaryBackend, "  模型: ", selectedModelStr)
}

// 哔哩哔哩链接解析
func checkParse(ctx *EasyBot.CQMessage) {
	reg := regexp.MustCompile(everyBiliLinkRegexp)
	matches := reg.FindAllStringSubmatch(ctx.GetRawMessageOrMessage(), -1)
	if len(matches) > 0 {

		if ctx.IsCardMsg() { // 屏蔽合并转发
			log.Debug("ctx.IsCardMsg()")
			cardMsg, err := ctx.ToCardMsg()
			if err == nil {
				if cardMsg.App == "com.tencent.multimsg" {
					log.Debug("cardMsg.App == \"com.tencent.multimsg\"")
					return
				}
			}
		}

		log.Debug("[parse] 识别到哔哩哔哩链接: ", matches[0][0])
		id, kind, summary, tts, upload := extractBiliLink(matches[0][0])
		if kind == "SHORT" { //短链先解析提取再往下, 保证parseHistory里没有短链
			loc := deShortLink(id)
			matches = reg.FindAllStringSubmatch(loc, -1)
			if len(matches) > 0 {
				log.Debug("[parse] 短链解析结果: ", matches[0][0])
				id, kind, _, _, _ = extractBiliLink(matches[0][0])
			} else {
				log.Debug("[parse] 短链解析失败: ", loc)
				return
			}
		}
		ctx.SendMsg(parseAndFormatBiliLink(ctx, id, kind, summary, tts, upload))
	}
}
