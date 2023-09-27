package main

import (
	"NothinBot/EasyBot"
	"regexp"
	"strings"
)

var (
	exp      = regexp.MustCompile(`[吗？\?]`)
	replacer = strings.NewReplacer("吗", "", "你", "我", "？", "！", "?", "!")
)

func checkAIReply2077(ctx *EasyBot.CQMessage) {
	if matches := exp.FindAllStringSubmatch(ctx.GetRawMessageOrMessage(), -1); ctx.IsToMe() && len(matches) > 0 {
		ctx.SendMsgReply(replacer.Replace(ctx.GetRawMessageOrMessage()))
	}
}
