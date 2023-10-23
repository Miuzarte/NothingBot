package main

import (
	"NothinBot/EasyBot"
	"strings"
)

var (
	SuperAiReplacer = strings.NewReplacer("吗", "", "你", "我", "是不是", "是", "？", "！", "?", "!")
)

func checkAIReply2077(ctx *EasyBot.CQMessage) {
	if matches := ctx.RegexpFindAllStringSubmatch(`[吗？\?]\s*$|是不是`); ctx.IsToMe() && len(matches) > 0 {
		ctx.SendMsgReply(ctx.StringsReplace(SuperAiReplacer))
	}
}
