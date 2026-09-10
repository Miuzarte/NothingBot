package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/redis/rueidis"
)

// 本文件的日志 scope
var logGroup = logger.New("Group")

const (
	GROUP_AT_REGEXP     = `(?i)谁(?:@|at|艾特)了?\s*(我|\[CQ:at,qq=(\d+).*?])\s*$`
	GROUP_RECALL_REGEXP = `(?i)\s*(我|\[CQ:at,qq=(\d+).*?])\s*撤回了什么$`
)

var (
	// [1]: "我" / "[CQ:at,qq=2393827810,name=@rurudoBOT]"
	// [2]: 2393827810

	groupAtReg     = regexp.MustCompile(GROUP_AT_REGEXP)
	groupRecallReg = regexp.MustCompile(GROUP_RECALL_REGEXP)
)

const groupMId ModuleId = "Group"

var (
	moduleGroupAt = ModuleMeta{
		Name:    groupMId.WithSuffix("At"),
		Desc:    "查看at记录",
		HelpMsg: "谁(@|at|艾特)了(我|[@])",
	}
	moduleGroupRecall = ModuleMeta{
		Name:    groupMId.WithSuffix("Recall"),
		Desc:    "查看撤回记录",
		HelpMsg: "(我|[@])撤回了什么",
	}
)

var moduleGroup = Module{
	ModuleMeta: ModuleMeta{
		Name:   groupMId,
		Hidden: true,
	},
	Disable: env.Testing,
	SubModules: []*ModuleMeta{
		&moduleGroupAt,
		&moduleGroupRecall,
	},
}

func init() {
	NoBuildPrintFile("M_Group.go")

	moduleGroup.Init = initGroup
	modules.Add(&moduleGroup)
}

func initGroup() {
	onebot.AddMatcher(moduleGroupAt.Name.String(), EasyOnebot.NewMatcher().
		OnTypes([]string{event.TYPE_L1_MESSAGE}, []string{event.TYPE_L2_MESSAGE_GROUP}).
		OnRegexpFindAllStringSubmatch(groupAtReg).
		Do(moduleGroup.RWMuWrap(func(ctx *EasyOnebot.Ctx) {
			ctxGroup(ctx, M_GROUP_AT)
		})),
	)
	onebot.AddMatcher(moduleGroupRecall.Name.String(), EasyOnebot.NewMatcher().
		OnTypes([]string{event.TYPE_L1_MESSAGE}, []string{event.TYPE_L2_MESSAGE_GROUP}).
		OnRegexpFindAllStringSubmatch(groupRecallReg).
		Do(moduleGroup.RWMuWrap(func(ctx *EasyOnebot.Ctx) {
			ctxGroup(ctx, M_GROUP_RECALL)
		})),
	)
}

const (
	M_GROUP_AT     = 1
	M_GROUP_RECALL = 2
)

// [TODO] 谁戳我
func ctxGroup(ctx *EasyOnebot.Ctx, op uint8) {
	var submatches []string
	switch op {
	case M_GROUP_AT:
		submatches = ctx.Submatches.Get(moduleGroupAt.Name.String())[0]
	case M_GROUP_RECALL:
		submatches = ctx.Submatches.Get(moduleGroupRecall.Name.String())[0]
	default:
		logGroup.Panic().
			Int("op", int(op)).
			Msg("invalid op")
	}

	var uin int
	if submatches[2] != "" {
		uin, _ = strconv.Atoi(submatches[2])
	}
	if uin == 0 {
		uin = ctx.Event.Sender.UserId
	}

	var uname string
	resp, err := ctx.Std.GetGroupMemberInfo(ctx.Event.GroupId, uin, false)
	if err != nil {
		uname = Itoa(uin)
	} else {
		uname = resp.Nickname
	}

	var msgList []event.MessageGroup
	switch op {
	case M_GROUP_AT:
		msgList, err = onebot.RedisRangeGetAtBy(ctx.Event.GroupId, uin)
	case M_GROUP_RECALL:
		msgList, err = onebot.RedisRangeGetRecall(ctx.Event.GroupId, uin)
	}
	if rueidis.IsRedisNil(err) {
		ctx.SendMsgReply("数据库中没有记录")
		return
	} else if err != nil {
		ctx.SendMsgf("数据库操作失败：%v", err)
	}
	slices.Reverse(msgList) // 使最旧的消息排最前

	var forward message.SegmentArray
	switch op {
	case M_GROUP_AT:
		if len(msgList) == 0 {
			ctx.SendMsgReplyf("数据库中没有%s被@的记录", uname)
			return
		}
		forward = message.SegmentArray{
			message.Node3(ctx.Event.SelfId, "", fmt.Sprintf("%s被@了%d次", uname, len(msgList))),
		}
	case M_GROUP_RECALL:
		if len(msgList) == 0 {
			ctx.SendMsgReplyf("数据库中没有%s的撤回记录", uname)
			return
		}
		forward = message.SegmentArray{
			message.Node3(ctx.Event.SelfId, "", fmt.Sprintf("%s撤回了%d条消息", uname, len(msgList))),
		}
	}
	forward[0].Data["time"] = uint(uint(msgList[0].Time) - uint(time.Hour*24/time.Second))

	for _, msg := range msgList {
		if msg.Message != nil {
			node := message.Node3Any(msg.Sender.UserId, msg.Sender.GetCardOrNickname(), msg.Message)
			node.Data["time"] = msg.Time
			forward.Append(node)
		}
	}
	_, err = ctx.SendForwardMsgAuto(forward)
	if err != nil {
		ctx.SendMsg("合并转发发送失败")
		logGroup.Error().
			Err(err).
			Msg("failed to send forward msg")
		dbg, _ := json.Marshal(forward)
		logGroup.Error().
			Str("content", string(dbg)).
			Msg("failed to send forward msg")
		return
	}
}
