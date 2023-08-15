package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	recallSwitch = true
)

func controlRecall(msg gocqMessage) {
	reg := regexp.MustCompile("(开启|启用|关闭|禁用)撤回记录").FindAllStringSubmatch(msg.message, -1)
	if matchSU(msg.user_id) && len(reg) != 0 {
		switch reg[0][1] {
		case "开启", "启用":
			recallSwitch = true
			sendMsgSingle(msg.user_id, 0, "撤回记录已启用")
		case "关闭", "禁用":
			recallSwitch = false
			sendMsgSingle(msg.user_id, 0, "撤回记录已禁用")
		}
	}
}

func formatRecall(id int, filter int, kind string) []map[string]any {
	forwardNode := []map[string]any{}
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
	if rcListLen >= 99 { //超过100条合并转发放不下, 标题占1条
		rcListLen = 99
	}
	forwardNode = append(forwardNode, func() map[string]any {
		switch kind {
		case "group":
			if filter != 0 {
				return map[string]interface{}{"type": "node", "data": map[string]interface{}{"name": "NothingBot", "uin": selfID,
					"content": fmt.Sprintf("群%d中%d的最近%d条被撤回的消息：", id, filter, rcListLen), "time": time.Now().Unix()}}
			} else {
				return map[string]interface{}{"type": "node", "data": map[string]interface{}{"name": "NothingBot", "uin": selfID,
					"content": fmt.Sprintf("群%d中最近%d条被撤回的消息：", id, rcListLen), "time": time.Now().Unix()}}
			}
		case "private":
			return map[string]any{"type": "node", "data": map[string]any{"name": "NothingBot", "uin": selfID,
				"content": fmt.Sprintf("%d的最近%d条被撤回的消息：", id, rcListLen), "time": time.Now().Unix()}}
		}
		return nil
	}())
	for i := 0; i < rcListLen; i++ {
		rcMsg := rcList[i]
		name := fmt.Sprintf(
			`(%s)%s%s`,
			rcMsg.timeF,
			func() string {
				if rcMsg.sender_card != "" {
					return rcMsg.sender_card
				}
				return rcMsg.sender_nickname
			}(),
			func() string {
				if rcMsg.operator_id != rcMsg.user_id {
					return "(他人撤回)"
				}
				return ""
			}())
		content := strings.ReplaceAll(rcMsg.messageF, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
		forwardNode = append(forwardNode, map[string]any{"type": "node", "data": map[string]any{"name": name, "uin": rcMsg.user_id,
			"content": content}})
	}
	return forwardNode
}

func checkRecallSend(msg gocqMessage) {
	if !recallSwitch {
		return
	}
	reg := regexp.MustCompile(`^让我康康(\s?\[CQ:at,qq=)?([0-9]{1,11})?(\]\s?)?撤回了什么$`).FindAllStringSubmatch(msg.message, -1)
	if len(reg) != 0 {
		var forwardNode []map[string]any
		switch msg.message_type {
		case "group":
			filter := func(reg string) int {
				if reg != "" {
					id, _ := strconv.Atoi(reg)
					return id
				}
				return 0
			}(reg[0][2])
			forwardNode = formatRecall(msg.group_id, filter, msg.message_type)
			sendForwardMsgSingle(0, msg.group_id, forwardNode)
		case "private":
			id := func(reg string) int {
				if reg != "" {
					id, _ := strconv.Atoi(reg)
					return id
				}
				return msg.user_id
			}(reg[0][2])
			if !matchSU(msg.user_id) && msg.user_id != id {
				sendMsgSingle(msg.user_id, 0, "👀？只有管理员才能查看他人的私聊撤回记录捏")
				sendMsg2SU(fmt.Sprintf("用户%s（%d）尝试查看的%d私聊撤回记录", msg.sender_nickname, msg.user_id, id))
				return
			}
			forwardNode = formatRecall(id, 0, msg.message_type)
			sendForwardMsgSingle(msg.user_id, 0, forwardNode)
		}
	}
	return
}
