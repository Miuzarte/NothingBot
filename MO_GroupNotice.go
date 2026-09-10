package main

import (
	"sync"

	"github.com/Miuzarte/EasyOnebot/api"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

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
		log.Panic("unreachable")
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
		log.Warnf("failed to get stranger info for user %d: %v", userId, uErr)
		return
	}
	if operatorId != 0 {
		if opErr != nil || opSi == nil {
			log.Warnf("failed to get stranger info for operator %d: %v", operatorId, opErr)
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
		log.Warn("handleGroupMemberChange: unknown sub_type: ", subType)
		return
	}

	_, err := onebot.Call().Std.SendGroupMsg(groupId, msg)
	if err != nil {
		log.Warn("failed to send group member change notification")
	}
}
