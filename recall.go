package main

import (
	"NothinBot/EasyBot"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

var recallEnable = false

type recall struct {
	kind      string //group / private
	id        int    //qq / groupId
	filter    int    //group member
	rcList    []*EasyBot.CQMessage
	rcListLen int
}

func initRecall() {
	recallEnable = v.GetBool("recall.enable")
}

// 获取
func (r *recall) get() *recall {
	var rcList []*EasyBot.CQMessage
	table := func() (table map[int]*EasyBot.CQMessage) {
		switch r.kind {
		case "group":
			if table = bot.MessageTableGroup[r.id]; table != nil {
				return
			}
		case "private":
			if table = bot.MessageTablePrivate[r.id]; table != nil {
				return
			}
		}
		return nil
	}()
	if table == nil {
		return r
	}
	for _, msg := range table {
		if msg.Extra.Recalled { //获取已撤回的消息
			if r.filter == 0 { //不指定群员时获取所有
				rcList = append(rcList, msg)
			} else {
				if msg.UserID == r.filter {
					rcList = append(rcList, msg)
				}
			}
		}
	}
	sort.Slice(rcList, func(i, j int) bool { //根据msg的时间戳由大到小排序
		return rcList[i].Event.Time > rcList[j].Event.Time
	})
	r.rcList = rcList
	r.rcListLen = len(rcList)
	return r
}

// 格式化
func (r *recall) format() (forwardMsg EasyBot.CQForwardMsg) {
	rcList := r.rcList
	rcListLen := r.rcListLen
	if rcListLen > 99 { //超过100条合并转发放不下, 标题占1条
		rcListLen = 99
	}
	forwardMsg = EasyBot.FastNewForwardMsg( //标题
		"NothingBot", bot.GetSelfID(), 0, 0,
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
	)
	for i := 0; i < rcListLen; i++ {
		rcMsg := rcList[i]
		name := fmt.Sprintf(
			`(%s)%s%s`,
			rcMsg.Extra.TimeFormat,
			rcMsg.GetCardOrNickname(),
			func() string {
				if rcMsg.Extra.OperatorID != rcMsg.UserID {
					return "(他人撤回)"
				}
				return ""
			}())
		content := strings.ReplaceAll(rcMsg.Extra.MessageWithReply, "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
		forwardMsg = EasyBot.AppendForwardMsg(forwardMsg, EasyBot.NewForwardNode(
			name, rcMsg.UserID, content, 0, 0))
	}
	return
}

// 撤回消息记录
func checkRecall(ctx *EasyBot.CQMessage) {
	//开关
	match := ctx.RegexpMustCompile(`(开启|启用|关闭|禁用)撤回记录`)
	if len(match) > 0 && ctx.IsPrivateSU() {
		switch match[0][1] {
		case "开启", "启用":
			recallEnable = true
			ctx.SendMsg("撤回记录已启用")
		case "关闭", "禁用":
			recallEnable = false
			ctx.SendMsg("撤回记录已禁用")
		}
		return
	}
	if !recallEnable && !ctx.IsSU() {
		return
	}
	//发送
	match = ctx.RegexpMustCompile(`^让我康康(\s*\[CQ:at,qq=)?([0-9]+)?(]\s*)?撤回了什么$`)
	if len(match) > 0 {
		r := recall{
			kind: ctx.MessageType,
			id: func() (id int) {
				switch {
				case ctx.IsPrivateSU():
					id = 0 //搜索所有
				case ctx.IsGroup():
					id = ctx.GroupID
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
		if !ctx.IsSU() && r.filter != ctx.UserID {
			ctx.SendMsg("👀？只有超级用户才能查看他人的私聊撤回记录捏")
			bot.Log2SU.Warn(fmt.Sprint("用户 ", ctx.Sender.NickName, "(", ctx.UserID, ") 尝试查看 ", r.id, " 的私聊撤回记录"))
			return
		}
		r.get()
		if r.rcListLen == 0 {
			if r.kind == "group" {
				if r.filter != 0 {
					ctx.SendMsgReply(fmt.Sprintf("%s之后群%d中的%d没有撤回过消息",
						time.Unix(startTime, 0).Format(timeLayout.M24C), r.id, r.filter))
				} else {
					ctx.SendMsgReply(fmt.Sprintf("%s之后群%d中没有人撤回过消息",
						time.Unix(startTime, 0).Format(timeLayout.M24C), r.id))
				}
			} else if r.kind == "private" {
				ctx.SendMsg(fmt.Sprintf("%s之后%d没有撤回过消息",
					time.Unix(startTime, 0).Format(timeLayout.M24C), r.id))
			}
			return
		}
		ctx.SendForwardMsg(r.format())
	}
}
