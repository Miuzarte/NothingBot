package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func formatAt(atID int, group int) []map[string]any {
	var forwardNode []map[string]any
	var atList []gocqMessage
	tables := func() []int {
		if group == 0 {
			var tables []int
			for i := range msgTableGroup {
				tables = append(tables, i)
			}
			return tables
		}
		return []int{group}
	}()
	for _, i := range tables {
		table := msgTableGroup[i]
		for _, msg := range table {
			for _, at := range msg.atWho {
				if atID == at {
					atList = append(atList, msg)
				}
			}
		}
	}
	sort.Slice(atList, func(i, j int) bool { //根据msg的时间戳由大到小排序
		return atList[i].time > atList[j].time
	})
	atListLen := len(atList)
	if atListLen > 99 { //超过100条合并转发放不下
		atListLen = 99
	}
	forwardNode = append(forwardNode, func() map[string]any {
		return map[string]any{"type": "node", "data": map[string]any{"name": "NothingBot", "uin": selfID,
			"content": func() string {
				if group != 0 {
					return fmt.Sprintf("群%d中最近%d条at过%d的消息：", group, atListLen, atID)
				} else {
					return fmt.Sprintf("所有群中最近%d条at过%d的消息：", atListLen, atID)
				}
			}(),
			"time": time.Now().Unix()}}
	}())
	for i := 0; i < atListLen; i++ {
		atMsg := atList[i]
		name := fmt.Sprintf(
			`(%s)%s%s%s`,
			atMsg.timeF,
			func() string {
				if atMsg.sender_card != "" {
					return atMsg.sender_card
				}
				return atMsg.sender_nickname
			}(),
			func() string {
				if group != 0 {
					return ""
				} else {
					return fmt.Sprintf("  (%d)", atMsg.group_id)
				}
			}(),
			func() string {
				if atMsg.recalled {
					if atMsg.operator_id == atMsg.user_id {
						return "(已撤回)"
					} else {
						return "(已被他人撤回)"
					}
				} else {
					return ""
				}
			}())
		content := strings.ReplaceAll(atMsg.messageF, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
		forwardNode = append(forwardNode, map[string]any{"type": "node", "data": map[string]any{"name": name, "uin": atMsg.user_id, "content": content}})
	}
	return forwardNode
}

func checkAt(msg gocqMessage) {
	reg := regexp.MustCompile(`^谁[aA艾]?[tT特]?@?(我|(\s?\[CQ:at,qq=)?([0-9]{1,11})?(\]\s?))$`).FindAllStringSubmatch(msg.message, -1)
	if len(reg) > 0 {
		var forwardNode []map[string]any
		var atID int
		if reg[0][1] == "我" {
			atID = msg.user_id
		} else {
			var err error
			atID, err = strconv.Atoi(reg[0][3])
			if err != nil {
				return
			}
		}
		switch msg.message_type {
		case "group":
			forwardNode = formatAt(atID, msg.group_id)
			sendForwardMsgSingle(0, msg.group_id, forwardNode)
		case "private":
			forwardNode = formatAt(atID, 0)
			sendForwardMsgSingle(msg.user_id, 0, forwardNode)
		}
	}
}
