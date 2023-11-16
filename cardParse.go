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
		matches := ctx.Unescape().RegexpFindAllStringSubmatch(`\[CQ:json,data=(\{.*\})\]`)
		var g gson.JSON
		if len(matches) > 0 {
			log.Debug("matches[0][1]: ", matches[0][1])
			g = gson.NewFrom(matches[0][1])
			if url := g.Get("meta.news.jumpUrl").Str(); !g.Get("meta.news.jumpUrl").Nil() {
				ctx.SendForwardMsg(
					EasyBot.NewForwardMsg(
						EasyBot.NewMsgForwardNode(ctx.MessageID),
						EasyBot.NewCustomForwardNode(
							"NotingBot_CardParse",
							bot.GetSelfID(),
							url, 0, 0)))
			} else if url := g.Get("meta.detail_1.qqdocurl").Str(); !g.Get("meta.detail_1.qqdocurl").Nil() {
				ctx.SendForwardMsg(
					EasyBot.NewForwardMsg(
						EasyBot.NewMsgForwardNode(ctx.MessageID),
						EasyBot.NewCustomForwardNode(
							"NotingBot_CardParse",
							bot.GetSelfID(),
							url, 0, 0)))
			}
		}
	}
}
