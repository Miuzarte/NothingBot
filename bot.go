package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var callSUMsgList []gocqMessage

func checkBotInternal(ctx gocqMessage) { //Bot内置逻辑
	var reg [][]string
	//连续at两次获取帮助, 带文字则视为喊话超级用户
	reg = regexp.MustCompile(fmt.Sprintf(`^\[CQ:at\,qq=%d]\s*\[CQ:at\,qq=%d]\s*(.*)$`, selfID, selfID)).FindAllStringSubmatch(ctx.message, -1)
	if len(reg) > 0 {
		call := reg[0][1]
		if len(call) > 0 { //记录喊话
			callSUMsgList = append(callSUMsgList, ctx)
			sendMsgReplyCTX(ctx, "[NothingBot] 已记录此条喊话并通知超级用户，需要帮助列表请在一条消息内仅at我两次")
			log2SU.Info("收到一条新的喊话")
		} else { //输出帮助
			sendMsgCTX(ctx, "[NothingBot] [Help] working...")
		}
	}
	//发送收件箱
	reg = regexp.MustCompile(`^(喊话列表|收件箱)$`).FindAllStringSubmatch(ctx.message, -1)
	if len(reg) > 0 && ctx.message_type == "private" && matchSU(ctx.user_id) {
		sort.Slice(callSUMsgList, func(i, j int) bool { //根据msg的时间戳由大到小排序
			return callSUMsgList[i].time > callSUMsgList[j].time
		})
		callSUMsgLen := len(callSUMsgList)
		if callSUMsgLen > 99 { //超过100条合并转发放不下
			callSUMsgLen = 99
		}
		var forwardNode []map[string]any
		for i := 0; i < callSUMsgLen; i++ {
			callSUMsg := callSUMsgList[i]
			name := fmt.Sprintf(
				`(%s)%s  (%d)`,
				callSUMsg.timeF,
				func() string {
					if callSUMsg.sender_card != "" {
						return callSUMsg.sender_card
					}
					return callSUMsg.sender_nickname
				}(),
				callSUMsg.group_id)
			content := strings.ReplaceAll(callSUMsg.messageF, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
			forwardNode = append(forwardNode, map[string]any{"type": "node", "data": map[string]any{"name": name, "uin": callSUMsg.user_id, "content": content}})
		}
	}
}
