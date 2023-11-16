package main

import (
	"NothinBot/EasyBot"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type whoAtMe struct {
	groupId   int
	atId      int
	atList    []*EasyBot.CQMessage
	atListLen int
}

// 获取
func (w *whoAtMe) get() *whoAtMe {
	atList := []*EasyBot.CQMessage{}
	tables := func() []int {
		if w.groupId == 0 {
			var tables []int
			for i := range bot.MessageTableGroup {
				tables = append(tables, i)
			}
			return tables
		}
		return []int{w.groupId}
	}()
	for _, i := range tables {
		table := bot.MessageTableGroup[i]
		for _, msg := range table {
			for _, at := range msg.Extra.AtWho {
				if w.atId == at {
					atList = append(atList, msg)
				}
			}
		}
	}
	sort.Slice(atList, func(i, j int) bool { //根据msg的时间戳由大到小排序
		return atList[i].Event.Time > atList[j].Event.Time
	})
	w.atList = atList
	w.atListLen = len(atList)
	return w
}

// 格式化
func (w *whoAtMe) format() (forwardMsg EasyBot.CQForwardMsg) {
	atList := w.atList
	atListLen := len(atList)
	if atListLen > 99 { //超过100条合并转发放不下, 标题占1条
		atListLen = 99
	}
	forwardMsg = EasyBot.NewForwardMsg(EasyBot.NewCustomForwardNode( //标题
		"NothingBot",
		bot.GetSelfID(),
		func() string {
			if w.groupId != 0 {
				return fmt.Sprintf("%s之后群%d中最近%d条at过%d的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), w.groupId, atListLen, w.atId)
			} else {
				return fmt.Sprintf("%s之后所有群中最近%d条at过%d的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), atListLen, w.atId)
			}
		}(),
		0, 0,
	))
	for i := 0; i < atListLen; i++ {
		atMsg := w.atList[i]
		name := fmt.Sprintf(`[%s]%s%s%s`,
			atMsg.Extra.TimeFormat,
			atMsg.GetCardOrNickname(),
			func() string {
				if w.groupId != 0 {
					return ""
				} else { //查看所有群的时候补充来源群
					return fmt.Sprintf("  (%d)", atMsg.GroupID)
				}
			}(),
			func() string {
				if atMsg.Extra.Recalled {
					if atMsg.Extra.OperatorID == atMsg.UserID {
						return "(已撤回)"
					} else {
						return "(已被他人撤回)"
					}
				} else {
					return ""
				}
			}())
		uin := func() (uin int) {
			if atMsg.UserID != 0 {
				return atMsg.UserID
			}
			return bot.GetSelfID()
		}()
		content := strings.ReplaceAll(atMsg.Extra.MessageWithReply, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
		forwardMsg = EasyBot.AppendForwardMsg(forwardMsg, EasyBot.NewCustomForwardNode(
			name, uin, content, 0, 0))
	}
	return
}

// 谁at我
func checkWhoAtMe(ctx *EasyBot.CQMessage) {
	match := ctx.RegexpFindAllStringSubmatch(`^谁(@|[aA艾][tT特])(我|(\s*\[CQ:at,qq=)?([0-9]{1,11})?(]\s*))$`)
	if len(match) > 0 {
		var atId int
		if match[0][2] == "我" {
			atId = ctx.UserID
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
				if ctx.MessageType == "group" {
					return ctx.GroupID
				}
				return 0
			}(),
		}
		w.get()
		if w.atListLen == 0 {
			if w.groupId != 0 {
				ctx.SendMsgReply(fmt.Sprintf("%s之后群%d中没有人at过%d", time.Unix(startTime, 0).Format(timeLayout.M24C), w.groupId, w.atId))
			} else {
				ctx.SendMsgReply(fmt.Sprintf("%s之后所有群中没有人at过%d", time.Unix(startTime, 0).Format(timeLayout.M24C), w.atId))
			}
			return
		}
		ctx.SendForwardMsg(w.format())
	}
}
