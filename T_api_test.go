package main

import (
	"testing"
)

func TestApiGetMsg(t *testing.T) {
	InitBot().Run()
	resp, err := onebot.Call().Std.GetMsg(966916846)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%+v", resp)
	// receive: {"status":"ok","retcode":0,"data":{"time":1738937558,"message_type":"group","message_id":966916846,"real_id":966916846,"sender":{"user_id":982809597,"nickname":"\u8B2C\u7D17\u7279\u2067\u309A\u309A\u309A\u309A\u309A\u309A\u309A\u309A","sex":"unknown"},"message":[{"type":"text","data":{"text":"23333"}}]},"echo":"get_msg_1738937711960557500"}

	onebot.Stop()
}

func TestGetGroupList(t *testing.T) {
	InitBot().Run()
	resp, err := onebot.Call().Std.GetGroupList()
	if err != nil {
		t.Error(err)
	}
	t.Logf("%+v", resp)

	onebot.Stop()
}

func TestGetFriendList(t *testing.T) {
	InitBot().Run()
	resp, err := onebot.Call().Std.GetFriendList()
	if err != nil {
		t.Error(err)
	}
	t.Logf("%+v", resp)

	onebot.Stop()
}

func TestGetMsg(t *testing.T) {
	const msgId = 163059167
	InitBot().Run()
	resp, err := onebot.Call().Std.GetMsg(msgId)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%+v", resp)
	for i, seg := range resp.Message {
		t.Logf("%d - %+v", i, seg)
	}
}

func TestGetForwardMsg(t *testing.T) {
	const forwardId = `aEGUZVEr+8sX6ZQzrmg14PkJJGZnfNU5LE4fXkEnVlBoEmDBTzyyoY/e/VIWknU6`
	InitBot().Run()
	resp, err := onebot.Call().Std.GetForwardMsg(forwardId)
	if err != nil {
		t.Error(err)
	}
	// t.Logf("%+v", resp)
	for _, r := range resp.Message {
		t.Logf("%+v", r)
	}

	onebot.Stop()
}
