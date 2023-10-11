package main

import (
	"NothinBot/EasyBot"
	"strings"
)

var (
	replacer = strings.NewReplacer("吗", "", "你", "我", "？", "！", "?", "!")
)

func checkAIReply2077(ctx *EasyBot.CQMessage) {
	if matches := ctx.RegexpMustCompile(`[吗？\?]\s*$`); ctx.IsToMe() && len(matches) > 0 {
		ctx.SendMsgReply(replacer.Replace(ctx.GetRawMessageOrMessage()))
	}
}
