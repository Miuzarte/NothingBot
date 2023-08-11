package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

func getDynamicJson(dynamicID string) gson.JSON { //获取动态数据
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/polymer/web-dynamic/v1/detail").
		WithAddQuery("id", dynamicID).WithHeaders(iheaders).WithCookie(cookie).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getDynamicJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawDynamicJson:", body)
	dynamicJson := gson.NewFrom(body)
	if dynamicJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 动态", dynamicID, "信息获取错误:", body)
	}
	return dynamicJson
}

func getVoteJson(voteID string) gson.JSON { //.Get("data.info")
	body := ihttp.New().WithUrl("https://api.vc.bilibili.com/vote_svr/v1/vote_svr/vote_info").
		WithAddQuery("vote_id", voteID).WithHeaders(iheaders).WithCookie(cookie).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getVoteJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawVoteJson:", body)
	voteJson := gson.NewFrom(body)
	if voteJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 投票", voteID, "信息获取错误:", body)
	}
	return voteJson
}

func formatDynamic(g gson.JSON) string { //主动态"data.item", 转发原动态"data.item.orig"
	truncationLength := v.GetInt( //简介截断长度
		"parse.settings.descTruncationLength")
	live := gson.NewFrom(g.Get( //直播
		"modules.module_dynamic.major.live_rcmd.content").Str())
	dynamic := g.Get("modules.module_dynamic")               //动态
	author := g.Get("modules.module_author")                 //发布
	draw := g.Get("modules.module_dynamic.major.draw")       //图片
	archive := g.Get("modules.module_dynamic.major.archive") //视频
	article := g.Get("modules.module_dynamic.major.article") //文章
	id := g.Get("id_str").Str()
	name := g.Get("modules.module_author.name").Str()
	additionalType := dynamic.Get("additional.type").Str() //动态子项类型
	appendReserve := func(g gson.JSON) string {            //预约格式化
		return fmt.Sprintf(
			`%s
%s
%s`,
			g.Get("title").Str(),
			g.Get("desc1.text").Str(), //"预计xxx发布"
			g.Get("desc2.text").Str()) //"xx人预约"/"xx观看"
	}
	appendVote := func(g gson.JSON) string { //投票格式化
		name := g.Get("name").Str()        //发起者
		title := g.Get("title").Str()      //标题
		desc := func(desc string) string { //简介
			if (desc != "" && desc != "-") && truncationLength > 0 {
				if len([]rune(desc)) > truncationLength {
					return fmt.Sprintf("\n简介：%s......", string([]rune(desc)[0:truncationLength]))
				} else {
					return fmt.Sprintf("\n简介：%s", desc)
				}
			}
			return ""
		}(g.Get("desc").Str())
		startTime, endTime := func(timeS1 int64, timeS2 int64) (string, string) {
			time1 := time.Unix(timeS1, 0)
			time2 := time.Unix(timeS2, 0)
			timeNow := time.Unix(time.Now().Unix(), 0)
			if time2.Format("2006") == timeNow.Format("2006") { //结束日期同年 不显示年份
				if time2.Format("01") == timeNow.Format("01") { //结束日期同月 不显示月份
					return time1.Format(timeLayout.M24), time2.Format(timeLayout.S24)
				}
				return time1.Format(timeLayout.M24), time2.Format(timeLayout.M24)
			}
			return time1.Format(timeLayout.L24), time2.Format(timeLayout.L24)
		}(int64(g.Get("starttime").Int()), int64(g.Get("endtime").Int()))
		c_cnt := g.Get("choice_cnt").Int()           //最大选择数
		cnt := g.Get("cnt").Int()                    //参与数
		option := func(options []gson.JSON) string { //图片
			var option string
			for _, j := range options {
				if !j.Get("cnt").Nil() {
					option += fmt.Sprintf("\n%d. %s  %d人选择",
						j.Get("idx").Int(),  //序号
						j.Get("desc").Str(), //描述
						j.Get("cnt").Int())  //选择数
				} else {
					option += fmt.Sprintf("\n%d. %s",
						j.Get("idx").Int(),  //序号
						j.Get("desc").Str()) //描述
				}
			}
			return option
		}(g.Get("options").Arr())
		return fmt.Sprintf(
			`%s发起的投票：%s%s
%s  -  %s
最多选%d项  %d人参与%s`,
			name, title, desc,
			startTime, endTime,
			c_cnt, cnt, option)
	}
	dynamicType := g.Get("type").Str() //动态类型
	log.Debugln("[bilibili] dynamicType:", dynamicType)
	switch dynamicType {
	case "DYNAMIC_TYPE_FORWARD": //转发
		topic := func(exist bool, topic string) string { //话题
			if exist {
				return fmt.Sprintf("\n#%s#", topic)
			}
			return ""
		}(!dynamic.Get("topic.name").Nil(), dynamic.Get("topic.name").Str())
		text := dynamic.Get("desc.text").Str() //文本
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：转发动态%s
%s

%s`,
			id,
			name, topic,
			text,
			formatDynamic(g.Get("orig")))
	case "DYNAMIC_TYPE_NONE": //转发的动态已删除
		return g.Get("modules.module_dynamic.major.none.tips").Str() //错误提示: "源动态已被作者删除"
	case "DYNAMIC_TYPE_WORD": //文本
		topic := func(exist bool, topic string) string { //话题
			if exist {
				return fmt.Sprintf("\n#%s#", topic)
			}
			return ""
		}(!dynamic.Get("topic.name").Nil(), dynamic.Get("topic.name").Str())
		text := dynamic.Get("desc.text").Str()                      //文本
		reserve := func(exist bool, reserveJson gson.JSON) string { //预约
			if exist {
				return fmt.Sprintf("\n%s", appendReserve(reserveJson))
			}
			return ""
		}(additionalType == "ADDITIONAL_TYPE_RESERVE", dynamic.Get("additional.reserve"))
		vote := func(exist bool, g gson.JSON) string { //投票
			if exist {
				voteJson := getVoteJson(strconv.Itoa(g.Get("vote_id").Int())).Get("data.info")
				return fmt.Sprintf("\n%s", appendVote(voteJson))
			}
			return ""
		}(additionalType == "ADDITIONAL_TYPE_VOTE", dynamic.Get("additional.vote"))
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s
%s%s%s`,
			id,
			name, topic,
			text, vote, reserve)
	case "DYNAMIC_TYPE_DRAW": //图文
		images := func(imgUrls []gson.JSON) string { //图片
			var images string
			for _, j := range imgUrls {
				images += fmt.Sprintf("[CQ:image,file=%s]", j.Get("src").Str())
			}
			return images
		}(draw.Get("items").Arr())
		topic := func(exist bool, topic string) string { //话题
			if exist {
				return fmt.Sprintf("\n#%s#", topic)
			}
			return ""
		}(!dynamic.Get("topic.name").Nil(), dynamic.Get("topic.name").Str())
		text := dynamic.Get("desc.text").Str()                      //文本
		reserve := func(exist bool, reserveJson gson.JSON) string { //预约
			if exist {
				return fmt.Sprintf("\n%s", appendReserve(reserveJson))
			}
			return ""
		}(additionalType == "ADDITIONAL_TYPE_RESERVE", dynamic.Get("additional.reserve"))
		vote := func(exist bool, g gson.JSON) string { //投票
			if exist {
				voteJson := getVoteJson(strconv.Itoa(g.Get("vote_id").Int())).Get("data.info")
				return fmt.Sprintf("\n%s", appendVote(voteJson))
			}
			return ""
		}(additionalType == "ADDITIONAL_TYPE_VOTE", dynamic.Get("additional.vote"))
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s
%s
%s%s%s`,
			id,
			name, topic,
			text,
			images, vote, reserve)
	case "DYNAMIC_TYPE_AV": //视频
		action := author.Get("pub_action").Str()         //"投稿了视频"/"发布了动态视频"
		topic := func(exist bool, topic string) string { //话题
			if exist {
				return fmt.Sprintf("\n#%s#", topic)
			}
			return ""
		}(!dynamic.Get("topic.name").Nil(), dynamic.Get("topic.name").Str())
		text := func(exist bool, text string) string { //文本
			if text == archive.Get("desc").Str() { //如果文本和简介相同，不显示文本
				return ""
			}
			if exist {
				return fmt.Sprintf("\n%s", text)
			}
			return ""
		}(!dynamic.Get("desc.text").Nil(), dynamic.Get("desc.text").Str())
		cover := archive.Get("cover").Str() //封面
		aid := archive.Get("aid").Str()     //av号数字
		title := archive.Get("title").Str() //标题
		desc := func(desc string) string {  //简介
			if (desc != "" && desc != "-") && truncationLength > 0 {
				if len([]rune(desc)) > truncationLength {
					return fmt.Sprintf("\n简介：%s......", string([]rune(desc)[0:truncationLength]))
				} else {
					return fmt.Sprintf("\n简介：%s", desc)
				}
			}
			return ""
		}(archive.Get("desc").Str())
		play := archive.Get("stat.play").Str()       //再生
		danmaku := archive.Get("stat.danmaku").Str() //弹幕
		bvid := archive.Get("bvid").Str()            //bv号
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s%s%s

[CQ:image,file=%s]
av%s
%s%s
%s播放  %s弹幕
www.bilibili.com/video/%s`,
			id,
			name, action, topic, text,
			cover,
			aid,
			title, desc,
			play, danmaku,
			bvid)
	case "DYNAMIC_TYPE_ARTICLE": //文章
		action := author.Get("pub_action").Str()     //"投稿了文章"
		covers := func(imgUrls []gson.JSON) string { //封面组
			var images string
			for _, j := range imgUrls {
				images += fmt.Sprintf("[CQ:image,file=%s]", j.Str())
			}
			return images
		}(article.Get("covers").Arr())
		cvid := article.Get("id").Int()     //cv号数字
		title := article.Get("title").Str() //标题
		label := article.Get("label").Str() //xxx阅读
		desc := article.Get("desc").Str()   //简介
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s
%s
cv%d
%s
%s
%s
www.bilibili.com/read/cv%d`,
			id,
			name, action,
			covers,
			cvid,
			title,
			label,
			desc,
			cvid)
	case "DYNAMIC_TYPE_LIVE_RCMD": //直播（动态流拿不到）
		action := author.Get("pub_action").Str()                             //"直播了"
		cover := live.Get("live_play_info.cover").Str()                      //封面
		title := live.Get("live_play_info.title").Str()                      //房间名
		parea := g.Get("live_play_info.parent_area_name").Str()              //主分区
		sarea := g.Get("live_play_info.area_name").Str()                     //子分区
		whatched := live.Get("live_play_info.watched_show.text_large").Str() //xxx人看过
		roomID := live.Get("live_play_info.room_id").Int()                   //房间号
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s
[CQ:image,file=%s]
%s
%s - %s
%s
live.bilibili.com/%d`,
			id,
			name, action,
			cover,
			title,
			parea, sarea,
			whatched,
			roomID)
	case "DYNAMIC_TYPE_COMMON_SQUARE": //应用装扮同步动态
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：

这是一条应用装扮同步动态：%s`,
			id,
			name,
			dynamicType)
	}
	log.Errorln("[bilibili] 未知的动态类型:", dynamicType, id)
	sendMsg2Admin("[bilibili] 未知的动态类型：" + dynamicType + "\n" + id)
	return fmt.Sprintf(
		`t.bilibili.com/%s
%s：

未知的动态类型：%s`,
		id,
		name,
		dynamicType)
}

func getArchiveJsonA(aid string) gson.JSON { //.Get("data"))
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/view").
		WithAddQuery("aid", aid).WithHeaders(iheaders).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getArchiveJsonA().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawVideoJsonA", body)
	videoJson := gson.NewFrom(body)
	if videoJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 视频", aid, "信息获取错误:", body)
	}
	return videoJson
}

func getArchiveJsonB(bvid string) gson.JSON { //.Get("data"))
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/view").
		WithAddQuery("bvid", bvid).WithHeaders(iheaders).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getArchiveJsonB().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawVideoJsonB", body)
	videoJson := gson.NewFrom(body)
	if videoJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 视频", bvid, "信息获取错误:", body)
	}
	return videoJson
}

func formatArchive(g gson.JSON) string {
	truncationLength := v.GetInt( //简介截断长度
		"parse.settings.descTruncationLength")
	pic := g.Get("pic").Str()          //封面
	aid := g.Get("aid").Int()          //av号数字
	title := g.Get("title").Str()      //标题
	up := g.Get("owner.name").Str()    //up主
	desc := func(desc string) string { //简介
		if (desc != "" && desc != "-") && truncationLength > 0 {
			if len([]rune(desc)) > truncationLength {
				return fmt.Sprintf("\n简介：%s......", string([]rune(desc)[0:truncationLength]))
			} else {
				return fmt.Sprintf("\n简介：%s", desc)
			}
		}
		return ""
	}(g.Get("desc").Str())
	view := g.Get("stat.view").Int()       //再生
	danmaku := g.Get("stat.danmaku").Int() //弹幕
	reply := g.Get("stat.reply").Int()     //回复
	like := g.Get("stat.like").Int()       //点赞
	coin := g.Get("stat.coin").Int()       //投币
	favor := g.Get("stat.favorite").Int()  //收藏
	bvid := g.Get("bvid").Str()            //bv号
	return fmt.Sprintf(
		`[CQ:image,file=%s]
av%d
%s
UP：%s%s
%d播放  %d弹幕  %d回复
%d点赞  %d投币  %d收藏
www.bilibili.com/video/%s`,
		pic,
		aid,
		title,
		up, desc,
		view, danmaku, reply,
		like, coin, favor,
		bvid)
}

func getArticleJson(cvid string) gson.JSON { //.Get("data")
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/article/viewinfo").
		WithAddQuery("id", cvid).WithHeaders(iheaders).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getArticleJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawArticleJson:", body)
	articleJson := gson.NewFrom(body)
	if articleJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 文章", cvid, "信息获取错误:", body)
	}
	return articleJson
}

func formatArticle(g gson.JSON, cvid string) string { //文章信息拿不到自己的cv号
	images := func(imgUrls []gson.JSON) string { //头图
		var images string
		for _, j := range imgUrls {
			images += fmt.Sprintf("[CQ:image,file=%s]", j.Str())
		}
		return images
	}(g.Get("image_urls").Arr())
	title := g.Get("title").Str()          //标题
	author := g.Get("author_name").Str()   //作者
	view := g.Get("stats.view").Int()      //阅读
	reply := g.Get("stats.reply").Int()    //回复
	share := g.Get("stats.share").Int()    //分享
	like := g.Get("stats.like").Int()      //点赞
	coin := g.Get("stats.coin").Int()      //投币
	favor := g.Get("stats.favorite").Int() //收藏
	return fmt.Sprintf(
		`%s
cv%s
%s
作者：%s
%d阅读  %d回复  %d分享
%d点赞  %d投币  %d收藏
www.bilibili.com/read/cv%s`,
		images,
		cvid,
		title,
		author,
		view, reply, share,
		like, coin, favor,
		cvid)
}

func getSpaceJson(uid string) gson.JSON { //.Get("data.card")
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/card").
		WithAddQuery("mid", uid).WithHeaders(iheaders).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getSpaceJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawSpaceJson:", body)
	spaceJson := gson.NewFrom(body)
	if spaceJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 空间", uid, "信息获取错误:", body)
	}
	return spaceJson
}

func formatSpace(g gson.JSON) string {
	face := g.Get("face").Str()                      //头像
	name := g.Get("name").Str()                      //用户名
	level := g.Get("level_info.current_level").Int() //账号等级
	pendant := func(name string, pid int) string {   //头像框
		if name != "" && pid != 0 {
			return fmt.Sprintf("\n头像框：%s（%d）", name, pid)
		}
		return ""
	}(g.Get("pendant.name").Str(), g.Get("pendant.pid").Int())
	sign := func(str string) string { //签名
		if str != "" {
			return fmt.Sprintf("\n签名：%s", str)
		}
		return ""
	}(g.Get("sign").Str())
	fol := g.Get("attention").Int() //关注
	fans := g.Get("fans").Int()     //粉丝
	mid := g.Get("mid").Str()       //uid
	return fmt.Sprintf(
		`[CQ:image,file=%s]
%s（LV%d）%s%s
%d关注  %d粉丝
space.bilibili.com/%s`,
		face,
		name, level, pendant, sign,
		fol, fans,
		mid)
}

func getRoomJsonUID(uid string) gson.JSON { //uid获取直播间数据  .Gets("data", strconv.Itoa(uid))
	body := ihttp.New().WithUrl("https://api.live.bilibili.com/room/v1/Room/get_status_info_by_uids").
		WithAddQuery("uids[]", uid).WithHeaders(iheaders).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getRoomJsonUID().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawRoomJson:", body)
	liveJson := gson.NewFrom(body)
	if liveJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 直播间(UID)", uid, "信息获取错误:", body)
	}
	return liveJson
}

func getRoomJsonRoomID(roomID string) gson.JSON { //房间号获取直播间数据（拿不到UP用户名）  .Get("data")
	body := ihttp.New().WithUrl("https://api.live.bilibili.com/room/v1/Room/get_info").
		WithAddQuery("room_id", roomID).WithHeaders(iheaders).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getRoomJsonRoomID().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawRoomJson:", body)
	liveJson := gson.NewFrom(body)
	if liveJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 直播间(RoomID)", roomID, "信息获取错误:", body)
	}
	return liveJson
}

func formatLive(g gson.JSON) string {
	cover := g.Get("cover_from_user").Str() //封面
	keyframe := g.Get("keyframe").Str()     //关键帧
	uname := g.Get("uname").Str()           //主播
	status := func(status int) string {     //
		switch status {
		case 0:
			return "（未开播）"
		case 1:
			return "（直播中）"
		case 2:
			return "（轮播中）"
		default:
			return "（未知状态）"
		}
	}(g.Get("live_status").Int())
	title := g.Get("title").Str()               //房间名
	parea := g.Get("area_v2_parent_name").Str() //主分区
	sarea := g.Get("area_v2_name").Str()        //子分区
	history := func(state int) string {         //bot记录
		switch state {
		case streamState.ONLINE:
			return fmt.Sprintf("\n机器人缓存的上一次开播时间：\n%s",
				time.Unix(liveStateList[g.Get("room_id").Str()].TIME, 0).Format(timeLayout.M24C))
		case streamState.OFFLINE:
			return fmt.Sprintf("\n机器人缓存的上一次下播时间：\n%s",
				time.Unix(liveStateList[g.Get("room_id").Str()].TIME, 0).Format(timeLayout.M24C))
		default:
			return ""
		}
	}(liveStateList[g.Get("room_id").Str()].STATE)
	roomID := g.Get("room_id").Int() //房间号
	return fmt.Sprintf(
		`[CQ:image,file=%s][CQ:image,file=%s]
%s的直播间%s
%s
%s - %s%s
live.bilibili.com/%d`,
		cover, keyframe,
		uname, status,
		title,
		parea, sarea, history,
		roomID)
}
