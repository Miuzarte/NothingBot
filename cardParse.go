package main

import (
	"NothinBot/EasyBot"

	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

// 卡片消息解析（拒绝小程序）
func checkCardParse(ctx *EasyBot.CQMessage) {
	if ctx.IsJsonMsg() {
		log.Debug("isJsonMsg")
		matches := ctx.Unescape().RegexpMustCompile(`\[CQ:json,data=(\{.*\})\]`)
		var g gson.JSON
		if len(matches) > 0 {
			log.Debug("matches: ", matches)
			log.Debug("matches[0][1]: ", matches[0][1])
			g = gson.NewFrom(matches[0][1])
			if url := g.Get("meta.news.jumpUrl").Str(); !g.Get("meta.news.jumpUrl").Nil() {
				ctx.SendMsgReply(url)
			}
		}
	}
}
