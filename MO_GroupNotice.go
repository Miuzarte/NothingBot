package main

import (
	"sync"

	"NothingBot_v4/logger"
	"github.com/Miuzarte/EasyOnebot/api"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

// 本文件的日志 scope
var logGroupNotice = logger.New("GroupNotice")

func init() {
	NoBuildPrintFile("MO_GroupNotice.go")

	onebot.OnNoticeGroupIncrease(func(ngi *event.NoticeGroupIncrease) {
		go handleGroupMemberChange(ngi)
	})
	onebot.OnNoticeGroupDecrease(func(ngd *event.NoticeGroupDecrease) {
		go handleGroupMemberChange(ngd)
	})
	onebot.OnNoticeNotifyPoke(func(nnp *event.NoticeNotifyPoke) {
		if nnp.TargetId != nnp.SelfId {
			return
		}
		if nnp.GroupId != 0 {
			onebot.Call().Lgr.GroupPoke(nnp.GroupId, nnp.UserId)
			// onebot.Call().Std.SendGroupMsg(nnp.GroupId, message.Poke_Lgr("1", "-1"))
		} else {
			onebot.Call().Lgr.FriendPoke(nnp.UserId)
			// onebot.Call().Std.SendPrivateMsg(nnp.UserId, message.Poke_Lgr("1", "-1"))
		}
	})
}

func handleGroupMemberChange(notice any) {
	var userId, groupId, operatorId int
	var subType string

	switch n := notice.(type) {
	case *event.NoticeGroupIncrease:
		userId = n.UserId
		groupId = n.GroupId
		operatorId = n.OperatorId
		subType = n.SubType
	case *event.NoticeGroupDecrease:
		userId = n.UserId
		groupId = n.GroupId
		operatorId = n.OperatorId
		subType = n.SubType
	default:
		logGroupNotice.Panic().Msg("unreachable")
	}

	var uSi *api.StrangerInfo
	var uErr error
	var opSi *api.StrangerInfo
	var opErr error

	wg := sync.WaitGroup{}
	wg.Go(func() { uSi, uErr = onebot.GetStrangerInfoTryCache(userId) })
	if operatorId != 0 {
		wg.Go(func() { opSi, opErr = onebot.GetStrangerInfoTryCache(operatorId) })
	}

	if uErr != nil || uSi == nil {
		logGroupNotice.Warn().
			Err(uErr).
			Int("user", userId).
			Msg("failed to get stranger info")
		return
	}
	if operatorId != 0 {
		if opErr != nil || opSi == nil {
			logGroupNotice.Warn().
				Err(opErr).
				Int("operator", operatorId).
				Msg("failed to get stranger info")
			return
		}
	}

	var msg message.Segment
	switch subType {
	case event.TYPE_L3_NOTICE_GROUP_INCREASE_APPROVE:
		msg = message.Textf("%s(%d) 加入了群", uSi.Nickname, userId)

	case event.TYPE_L3_NOTICE_GROUP_INCREASE_INVITE:
		msg = message.Textf("%s(%d) 被 %s(%d) 邀请入群", uSi.Nickname, userId, opSi.Nickname, operatorId)

	case event.TYPE_L3_NOTICE_GROUP_DECREASE_LEAVE:
		msg = message.Textf("%s(%d) 离开了群", uSi.Nickname, userId)

	case event.TYPE_L3_NOTICE_GROUP_DECREASE_KICK:
		msg = message.Textf("%s(%d) 被 %s(%d) 移出了群", uSi.Nickname, userId, opSi.Nickname, operatorId)

	default:
		logGroupNotice.Warn().
			Str("subType", subType).
			Msg("unknown sub_type")
		return
	}

	_, err := onebot.Call().Std.SendGroupMsg(groupId, msg)
	if err != nil {
		logGroupNotice.Warn().Msg("failed to send group member change notification")
	}
}
