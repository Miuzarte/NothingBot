package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

var recallSwitch = true

type recall struct {
	kind      string //group / private
	id        int    //qq / groupId
	filter    int    //group member
	rcList    []gocqMessage
	rcListLen int
}

// 获取
func (r *recall) get() *recall {
	var rcList []gocqMessage
	table := func() map[int]gocqMessage {
		switch r.kind {
		case "group":
			if msgTableGroup[r.id] != nil {
				return msgTableGroup[r.id]
			}
		case "private":
			if msgTableFriend[r.id] != nil {
				return msgTableFriend[r.id]
			}
		}
		return nil
	}()
	if table == nil {
		return r
	}
	for _, msg := range table {
		if msg.extra.recalled { //获取已撤回的消息
			if r.filter == 0 { //不指定群员时获取所有
				rcList = append(rcList, msg)
			} else {
				if msg.user_id == r.filter {
					rcList = append(rcList, msg)
				}
			}
		}
	}
	sort.Slice(rcList, func(i, j int) bool { //根据msg的时间戳由大到小排序
		return rcList[i].time > rcList[j].time
	})
	r.rcList = rcList
	r.rcListLen = len(rcList)
	return r
}

// 格式化
func (r *recall) format() (forwardNode []map[string]any) {
	rcList := r.rcList
	rcListLen := r.rcListLen
	if rcListLen > 99 { //超过100条合并转发放不下, 标题占1条
		rcListLen = 99
	}
	forwardNode = appendForwardNode(forwardNode, gocqNodeData{ //标题
		content: []string{
			func() string {
				if r.kind == "group" {
					if r.filter != 0 {
						return fmt.Sprintf("%s之后群%d中来自%d的最近%d条被撤回的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), r.id, r.filter, rcListLen)
					} else {
						return fmt.Sprintf("%s之后群%d中最近%d条被撤回的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), r.id, rcListLen)
					}
				} else if r.kind == "private" {
					return fmt.Sprintf("%s之后%d的最近%d条被撤回的消息：", time.Unix(startTime, 0).Format(timeLayout.M24C), r.id, rcListLen)
				}
				return ""
			}(),
		},
	})
	for i := 0; i < rcListLen; i++ {
		rcMsg := rcList[i]
		name := fmt.Sprintf(
			`(%s)%s%s`,
			rcMsg.extra.timeFormat,
			rcMsg.getCardOrNickname(),
			func() string {
				if rcMsg.extra.operator_id != rcMsg.user_id {
					return "(他人撤回)"
				}
				return ""
			}())
		content := strings.ReplaceAll(rcMsg.extra.messageWithReply, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			name:    name,
			uin:     rcMsg.user_id,
			content: []string{content},
		})
	}
	return
}

// 撤回消息记录
func checkRecall(ctx gocqMessage) {
	//开关
	match := ctx.regexpMustCompile(`(开启|启用|关闭|禁用)撤回记录`)
	if len(match) > 0 && ctx.isPrivateSU() {
		switch match[0][1] {
		case "开启", "启用":
			recallSwitch = true
			ctx.sendMsg("撤回记录已启用")
		case "关闭", "禁用":
			recallSwitch = false
			ctx.sendMsg("撤回记录已禁用")
		}
		return
	}
	if !recallSwitch && !ctx.isSU() {
		return
	}
	//发送
	match = ctx.regexpMustCompile(`^让我康康(\s*\[CQ:at,qq=)?([0-9]+)?(]\s*)?撤回了什么$`)
	if len(match) > 0 {
		r := recall{
			kind: ctx.message_type,
			id: func() (id int) {
				switch {
				case ctx.isPrivateSU():
					id = 0 //搜索所有
				case ctx.isGroup():
					id = ctx.group_id
				}
				return
			}(),
			filter: func() (filter int) {
				if match[0][2] == "" {
					filter = 0 //列出所有
				} else {
					filter, _ = strconv.Atoi(match[0][2])
				}
				return
			}(),
		}
		if !ctx.isSU() && r.filter != ctx.user_id {
			ctx.sendMsg("👀？只有超级用户才能查看他人的私聊撤回记录捏")
			log2SU.Warn(fmt.Sprint("用户 ", ctx.sender_nickname, "(", ctx.user_id, ") 尝试查看 ", r.id, " 的私聊撤回记录"))
			return
		}
		r.get()
		if r.rcListLen == 0 {
			if r.kind == "group" {
				if r.filter != 0 {
					ctx.sendMsgReply(fmt.Sprintf("%s之后群%d中的%d没有撤回过消息",
						time.Unix(startTime, 0).Format(timeLayout.M24C), r.id, r.filter))
				} else {
					ctx.sendMsgReply(fmt.Sprintf("%s之后群%d中没有人撤回过消息",
						time.Unix(startTime, 0).Format(timeLayout.M24C), r.id))
				}
			} else if r.kind == "private" {
				ctx.sendMsg(fmt.Sprintf("%s之后%d没有撤回过消息",
					time.Unix(startTime, 0).Format(timeLayout.M24C), r.id))
			}
			return
		}
		ctx.sendForwardMsg(r.format())
	}
}
