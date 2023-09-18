package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

// 卡片消息解析（拒绝小程序）
func checkCardParse(ctx *gocqMessage) {
	if ctx.isJsonMsg() {
		log.Debug("isJsonMsg")
		matches := ctx.unescape().regexpMustCompile(`\[CQ:json,data=(\{.*\})\]`)
		var g gson.JSON
		if len(matches) > 0 {
			log.Debug("matches: ", matches)
			log.Debug("matches[0][1]: ", matches[0][1])
			g = gson.NewFrom(matches[0][1])
			if url := g.Get("meta.news.jumpUrl").Str(); !g.Get("meta.news.jumpUrl").Nil() {
				ctx.sendMsgReply(url)
			}
		}
	}
}
