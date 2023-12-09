package main

import (
	"NothinBot/EasyBot"
	"regexp"

	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

var (
	urlPath = [...]string{
		"meta.news.jumpUrl",
		"meta.detail_1.qqdocurl",
	}
)

// 卡片消息解析（拒绝小程序）
func checkCardParse(ctx *EasyBot.CQMessage) {
	if ctx.IsJsonMsg() {
		log.Debug("isJsonMsg")
		matches := ctx.Unescape().RegFindAllStringSubmatch(regexp.MustCompile(`\[CQ:json,data=(\{.*})]`))
		var g gson.JSON
		if len(matches) > 0 {
			log.Debug("matches[0][1]: ", matches[0][1])
			g = gson.NewFrom(matches[0][1])
			url := ""
			for _, s := range urlPath {
				if !g.Get(s).Nil() {
					url = g.Get(s).Str()
					break
				}
			}
			if url != "" {
				ctx.SendForwardMsg(
					EasyBot.NewForwardMsg(
						EasyBot.NewMsgForwardNode(ctx.MessageID),
						EasyBot.NewCustomForwardNodeOSR(url),
					),
				)
			}
		}
	}
}
