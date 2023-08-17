package main

import (
	"fmt"
	"regexp"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

const biliSearchRegexp = `[Bb]搜(综合|全部|所有|视频|番剧|影视|直播|直播间|主播|专栏|话题|用户|相簿)[\s:：]?(.*)`

type searchType struct {
	ALL       string
	VIDEO     string
	BANGUMI   string
	FT        string
	LIVE      string
	LIVE_ROOM string
	LIVE_USER string
	ARTICLE   string
	TOPIC     string
	USER      string
	PHOTO     string
}

var searchTypes = searchType{
	ALL:       "ALL",
	VIDEO:     "video",
	BANGUMI:   "media_bangumi",
	FT:        "media_ft",
	LIVE:      "live",
	LIVE_ROOM: "live_room",
	LIVE_USER: "live_user",
	ARTICLE:   "article",
	TOPIC:     "topic",
	USER:      "bili_user",
	PHOTO:     "photo",
}

func formatSearch(g gson.JSON) string {
	return ""
}

func biliSearch(keyword string, kind string) string {
	log.Debug("[search] 开始搜索: ", kind, keyword)
	imageSize := func(imageSize int) string { //结果图片压缩尺寸
		if imageSize != 0 {
			return fmt.Sprintf("@%dh", imageSize)
		}
		return ""
	}(v.GetInt("search.settings.imageSize"))
	g := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/search/type").
		WithAddQuerys(map[string]string{"search_type": kind, "keyword": keyword}).WithHeaders(iheaders).WithCookie(cookie).
		Get().WithError(func(err error) { log.Error("[ihttp] 请求错误: ", err) }).ToGson()
	log.Trace("[search] body: ", g.JSON("", ""))
	if g.Get("code").Int() != 0 {
		return ""
	}
	switch kind {
	case searchTypes.ALL: //综合
	case searchTypes.VIDEO:
	case searchTypes.BANGUMI:
	case searchTypes.FT:
	case searchTypes.LIVE:
	case searchTypes.LIVE_ROOM:
	case searchTypes.LIVE_USER:
	case searchTypes.ARTICLE:
	case searchTypes.TOPIC:
	case searchTypes.USER: //用户
		result := func(results []gson.JSON) string {
			str := ""
			resultCount := len(results)
			for i := 0; i < resultCount; i++ {
				upic := results[i].Get("upic").String()   //头像
				uname := results[i].Get("uname").String() //昵称
				level := results[i].Get("level").Int()    //等级
				mid := results[i].Get("mid").Int()        //uid
				fans := results[i].Get("fans").Int()      //粉丝数
				videos := results[i].Get("videos").Int()  //视频数
				str += fmt.Sprintf(
					`
[CQ:image,file=https:%s%s]
%d. %s（LV%d）
%d
%d粉丝  %d投稿`,
					upic, imageSize,
					i+1, uname, level,
					mid,
					fans, videos)
				switch {
				case i+1 < resultCount && i+1 == 8 && resultCount == 20:
					str += fmt.Sprintf("\n\n显示%d/%d(+)个结果", i+1, resultCount)
					i = resultCount
				case i+1 < resultCount && i+1 == 8:
					str += fmt.Sprintf("\n\n显示%d/%d个结果", i+1, resultCount)
					i = resultCount
				case i+1 == resultCount:
					str += fmt.Sprintf("\n\n共%d个结果", i+1)
					i = resultCount
				}
			}
			return str
		}(g.Get("data.result").Arr())
		return fmt.Sprintf(
			`快捷搜索用户：%s
%s`,
			keyword,
			result)
	case searchTypes.PHOTO:
	}
	return ""
}

func checkSearch(msg gocqMessage) {
	result := regexp.MustCompile(biliSearchRegexp).FindAllStringSubmatch(msg.message, -1)
	if len(result) == 0 {
		return
	}
	message := func(keyword string, kind string) string {
		log.Debug("[search] 识别搜索: ", kind, keyword)
		return biliSearch(keyword, func(kind string) string {
			switch kind {
			case "视频":
				return searchTypes.VIDEO
			case "番剧":
				return searchTypes.BANGUMI
			case "影视":
				return searchTypes.FT
			case "直播":
				return searchTypes.LIVE
			case "直播间":
				return searchTypes.LIVE_ROOM
			case "主播":
				return searchTypes.LIVE_USER
			case "专栏":
				return searchTypes.ARTICLE
			case "话题":
				return searchTypes.TOPIC
			case "用户":
				return searchTypes.USER
			case "相簿":
				return searchTypes.PHOTO
			}
			return ""
		}(kind))
	}(result[0][2], result[0][1])
	switch msg.message_type {
	case "group":
		sendMsgSingle(0, msg.group_id, message)
	case "private":
		sendMsgSingle(msg.user_id, 0, message)
	}
}
