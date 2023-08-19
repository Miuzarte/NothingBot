package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var recallSwitch = true

func formatRecall(id int, filter int, kind string) []map[string]any {
	var forwardNode []map[string]any
	var rcList []gocqMessage
	table := func() map[int]gocqMessage {
		switch kind {
		case "group":
			return msgTableGroup[id]
		case "private":
			return msgTableFriend[id]
		}
		return nil
	}()
	for _, msg := range table {
		if msg.recalled { //获取已撤回的消息
			if filter != 0 {
				if msg.user_id == filter {
					rcList = append(rcList, msg)
				}
			} else {
				rcList = append(rcList, msg)
			}
		}
	}
	sort.Slice(rcList, func(i, j int) bool { //根据msg的时间戳由大到小排序
		return rcList[i].time > rcList[j].time
	})
	rcListLen := len(rcList)
	if rcListLen > 99 { //超过100条合并转发放不下, 标题占1条
		rcListLen = 99
	}
	forwardNode = appendForwardNode(forwardNode, gocqNodeData{ //标题
		content: []string{
			func(kind string) string {
				switch kind {
				case "group":
					if filter != 0 {
						return fmt.Sprintf("%s之后群%d中%d的最近%d条被撤回的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), id, filter, rcListLen)
					}
					return fmt.Sprintf("%s之后群%d中最近%d条被撤回的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), id, rcListLen)
				case "private":
					return fmt.Sprintf("%s之后%d的最近%d条被撤回的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), id, rcListLen)
				}
				return ""
			}(kind),
		},
	})
	for i := 0; i < rcListLen; i++ {
		rcMsg := rcList[i]
		name := fmt.Sprintf(
			`(%s)%s%s`,
			rcMsg.timeF,
			cardORnickname(rcMsg),
			func() string {
				if rcMsg.operator_id != rcMsg.user_id {
					return "(他人撤回)"
				}
				return ""
			}())
		content := strings.ReplaceAll(rcMsg.message, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			name:    name,
			uin:     rcMsg.user_id,
			content: []string{content},
		})
	}
	return forwardNode
}

func checkRecall(ctx gocqMessage) {
	//开关
	reg := regexp.MustCompile("(开启|启用|关闭|禁用)撤回记录").FindAllStringSubmatch(ctx.message, -1)
	if matchSU(ctx.user_id) && len(reg) != 0 {
		switch reg[0][1] {
		case "开启", "启用":
			recallSwitch = true
			sendMsgCTX(ctx, "撤回记录已启用")
		case "关闭", "禁用":
			recallSwitch = false
			sendMsgCTX(ctx, "撤回记录已禁用")
		}
		return
	}
	if !recallSwitch {
		return
	}
	//发送
	reg = regexp.MustCompile(`^让我康康(\s?\[CQ:at,qq=)?([0-9]{1,11})?(\]\s?)?撤回了什么$`).FindAllStringSubmatch(ctx.message, -1)
	if len(reg) != 0 {
		switch ctx.message_type {
		case "group": //群内使用filter为群成员
			filter := func(reg string) int {
				if reg != "" {
					id, _ := strconv.Atoi(reg)
					return id
				}
				return 0
			}(reg[0][2])
			sendGroupForwardMsg(ctx.group_id, formatRecall(ctx.group_id, filter, ctx.message_type))
		case "private": //私聊使用id为球球号/群号
			id := func(reg string) int {
				if reg != "" {
					id, _ := strconv.Atoi(reg)
					return id
				}
				return ctx.user_id
			}(reg[0][2])
			if !matchSU(ctx.user_id) && ctx.user_id != id {
				sendPrivateMsg(ctx.user_id, "👀？只有超级用户才能查看他人的私聊撤回记录捏")
				log2SU.Warn(fmt.Sprint("用户 ", ctx.sender_nickname, "(", ctx.user_id, ") 尝试查看 ", id, " 的私聊撤回记录"))
				return
			}
			if msgTableFriend[id] != nil {
				sendPrivateForwardMsg(ctx.user_id, formatRecall(id, 0, "private"))
			}
			if msgTableGroup[id] != nil {
				sendPrivateForwardMsg(ctx.user_id, formatRecall(id, 0, "group"))
			}
		}
	}
	return
}
