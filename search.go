package main

import (
	"fmt"
	"regexp"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
)

var biliSearchRegexp = struct {
	USER string
}{
	USER: `[Bb]搜(视频|番剧|影视|直播|直播间|主播|专栏|话题|用户|相簿)[\s:：]?(.*)`,
}

/**
视频：video
番剧：media_bangumi
影视：media_ft
直播间及主播：live
直播间：live_room
主播：live_user
专栏：article
话题：topic
用户：bili_user
相簿：photo
*/

func biliSearch(search string, kind string) string {
	var search_type string
	switch kind {
	case "视频":
	case "番剧":
	case "影视":
	case "直播":
	case "直播间":
	case "主播":
	case "专栏":
	case "话题":
	case "用户":
		search_type = "bili_user"
	case "相簿":
	}
	if search_type == "" {
		return ""
	}
	g := ihttp.New().
		WithUrl("https://api.bilibili.com/x/web-interface/search/type").
		WithAddQuerys(map[string]string{"search_type": search_type, "keyword": search}).
		WithHeaders(iheaders).WithCookie(cookie).
		Get().WithError(func(err error) { log.Errorln("[ihttp] 请求错误:", err) }).ToGson()
	resultCount := len(g.Get("data.result").Arr())
	log.Traceln(g)
	var message string
	for i := 0; i < resultCount; i++ {
		upic := fmt.Sprintf("%d. [CQ:image,file=https:%s]\n", i+1, g.Get(fmt.Sprintf("data.result.%d.upic", i)).String()) //头像
		mid := fmt.Sprintf("%d\n", g.Get(fmt.Sprintf("data.result.%d.mid", i)).Int())                                     //uid
		uname := fmt.Sprintf("%s", g.Get(fmt.Sprintf("data.result.%d.uname", i)).String())                                //昵称
		level := fmt.Sprintf("（LV%d）\n", g.Get(fmt.Sprintf("data.result.%d.level", i)).Int())                             //等级
		fans := fmt.Sprintf("%d粉丝  ", g.Get(fmt.Sprintf("data.result.%d.fans", i)).Int())                                 //粉丝数
		videos := fmt.Sprintf("%d投稿\n", g.Get(fmt.Sprintf("data.result.%d.videos", i)).Int())                             //视频数
		message += upic + mid + uname + level + fans + videos
		if i == resultCount-1 {
			message += fmt.Sprintf("\n共%d个结果", resultCount)
		} else {
			if i == 9-1 {
				message += fmt.Sprintf("\n共%d个结果，仅显示前9个", resultCount)
				break
			}
			message += "\n"
		}
	}
	return message
}

func checkSearch(msg gocqMessage) {
	result := regexp.MustCompile(biliSearchRegexp.USER).FindAllStringSubmatch(msg.message, -1)
	if len(result) == 0 {
		return
	}
	searchType := result[0][1]
	searchContent := result[0][2]
	log.Debugln("[search] 识别到搜索", searchType, searchContent)
	message := biliSearch(searchContent, searchType)
	switch msg.message_type {
	case "group":
		sendMsgSingle(0, msg.group_id, message)
	case "private":
		sendMsgSingle(msg.user_id, 0, message)
	}
}
