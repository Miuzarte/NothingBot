package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type whoAtMe struct {
	groupId   int
	atId      int
	atList    []gocqMessage
	atListLen int
}

// 获取
func (w *whoAtMe) get() *whoAtMe {
	atList := []gocqMessage{}
	tables := func() []int {
		if w.groupId == 0 {
			var tables []int
			for i := range msgTableGroup {
				tables = append(tables, i)
			}
			return tables
		}
		return []int{w.groupId}
	}()
	for _, i := range tables {
		table := msgTableGroup[i]
		for _, msg := range table {
			for _, at := range msg.extra.atWho {
				if w.atId == at {
					atList = append(atList, *msg)
				}
			}
		}
	}
	sort.Slice(atList, func(i, j int) bool { //根据msg的时间戳由大到小排序
		return atList[i].time > atList[j].time
	})
	w.atList = atList
	w.atListLen = len(atList)
	return w
}

// 格式化
func (w *whoAtMe) format() (forwardNode []map[string]any) {
	atList := w.atList
	atListLen := len(atList)
	if atListLen > 99 { //超过100条合并转发放不下, 标题占1条
		atListLen = 99
	}
	forwardNode = appendForwardNode(forwardNode, gocqNodeData{ //标题
		content: []string{
			func() string {
				if w.groupId != 0 {
					return fmt.Sprintf("%s之后群%d中最近%d条at过%d的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), w.groupId, atListLen, w.atId)
				} else {
					return fmt.Sprintf("%s之后所有群中最近%d条at过%d的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), atListLen, w.atId)
				}
			}(),
		},
	})
	for i := 0; i < atListLen; i++ {
		atMsg := w.atList[i]
		name := fmt.Sprintf(`(%s)%s%s%s`,
			atMsg.extra.timeFormat,
			atMsg.getCardOrNickname(),
			func() string {
				if w.groupId != 0 {
					return ""
				} else { //查看所有群的时候补充来源群
					return fmt.Sprintf("  (%d)", atMsg.group_id)
				}
			}(),
			func() string {
				if atMsg.extra.recalled {
					if atMsg.extra.operator_id == atMsg.user_id {
						return "(已撤回)"
					} else {
						return "(已被他人撤回)"
					}
				} else {
					return ""
				}
			}())
		uin := func() (uin int) {
			if atMsg.user_id != 0 {
				return atMsg.user_id
			}
			return selfId
		}()
		content := strings.ReplaceAll(atMsg.extra.messageWithReply, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			name:    name,
			uin:     uin,
			content: []string{content},
		})
	}
	return
}

// 谁at我
func checkAt(ctx *gocqMessage) {
	match := ctx.regexpMustCompile(`^谁(@|[aA艾][tT特])(我|(\s*\[CQ:at,qq=)?([0-9]{1,11})?(]\s*))$`)
	if len(match) > 0 {
		var atId int
		if match[0][2] == "我" {
			atId = ctx.user_id
		} else {
			var err error
			atId, err = strconv.Atoi(match[0][4])
			if err != nil {
				return
			}
		}
		w := whoAtMe{
			atId: atId,
			groupId: func() int {
				if ctx.message_type == "group" {
					return ctx.group_id
				}
				return 0
			}(),
		}
		w.get()
		if w.atListLen == 0 {
			if w.groupId != 0 {
				ctx.sendMsgReply(fmt.Sprintf("%s之后群%d中没有人at过%d", time.Unix(startTime, 0).Format(timeLayout.M24C), w.groupId, w.atId))
			} else {
				ctx.sendMsgReply(fmt.Sprintf("%s之后所有群中没有人at过%d", time.Unix(startTime, 0).Format(timeLayout.M24C), w.atId))
			}
			return
		}
		ctx.sendForwardMsg(w.format())
	}
}
