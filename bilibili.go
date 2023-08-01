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
		WithAddQuery("id", dynamicID).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getDynamicJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawDynamicJson:", body)
	dynamicJson := gson.NewFrom(body)
	if dynamicJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 动态", dynamicID, "信息获取错误:", body)
	}
	return dynamicJson
}

func getVoteJson(voteID string) gson.JSON { //.Get("data.info")
	body := ihttp.New().WithUrl("https://api.vc.bilibili.com/vote_svr/v1/vote_svr/vote_info").
		WithAddQuery("vote_id", voteID).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getVoteJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawVoteJson:", body)
	voteJson := gson.NewFrom(body)
	if voteJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 投票", voteID, "信息获取错误:", body)
	}
	return voteJson
}

func formatDynamic(json gson.JSON) string { //主动态"data.item", 转发原动态"data.item.orig"
	var head string
	var content string
	id := fmt.Sprintf("t.bilibili.com/%s\n", json.Get("id_str").Str())
	name := fmt.Sprintf("%s：\n", json.Get("modules.module_author.name").Str())
	head = id + name
	truncationLength := v.GetInt("parse.settings.descTruncationLength") //简介截断长度
	dynamicType := json.Get("type").Str()                               //动态类型
	dynamic := json.Get("modules.module_dynamic")                       //动态
	author := json.Get("modules.module_author")                         //发布
	draw := json.Get("modules.module_dynamic.major.draw")               //图片
	archive := json.Get("modules.module_dynamic.major.archive")         //视频
	article := json.Get("modules.module_dynamic.major.article")         //文章
	additionalType := dynamic.Get("additional.type").Str()              //动态子项类型 投票/预约
	vote := dynamic.Get("additional.vote")                              //投票
	reserve := dynamic.Get("additional.reserve")                        //预约
	live := gson.NewFrom(json.Get(                                      //直播
		"modules.module_dynamic.major.live_rcmd.content").Str())
	appendVote := func(voteID string) string { //投票格式化
		var content string
		rawVoteJson := getVoteJson(voteID)
		if rawVoteJson.Get("code").Int() != 0 {
			return "投票信息获取错误"
		}
		voteJson := rawVoteJson.Get("data.info")
		start := fmt.Sprintf("%s开始\n", //开始时间
			time.Unix(int64(voteJson.Get("starttime").Int()), 0).Format(timeLayout.L24C))
		end := fmt.Sprintf("%s结束\n", //结束时间
			time.Unix(int64(voteJson.Get("endtime").Int()), 0).Format(timeLayout.L24C))
		name := fmt.Sprintf("%s发起的投票：\n", voteJson.Get("name").Str())        //发起者
		title := fmt.Sprintf("%s\n", voteJson.Get("title").Str())            //标题
		c_cnt := fmt.Sprintf("最多选%d项    ", voteJson.Get("choice_cnt").Int()) //最大多选数
		cnt := fmt.Sprintf("%d人参与", voteJson.Get("cnt").Int())               //参与数
		op_cnt := voteJson.Get("options_cnt").Int()                          //选项数
		content += name + title
		desc := voteJson.Get("desc").Str() //简介
		if (desc != "<nil>" && desc != "-") && truncationLength > 0 {
			if len([]rune(desc)) > truncationLength {
				content += fmt.Sprintf("简介：%c......\n", []rune(desc)[0:truncationLength])
			} else {
				content += fmt.Sprintf("简介：%s\n", desc)
			}
		}
		content += start + end + c_cnt + cnt
		for i := 0; i < op_cnt; i++ { //选项序列
			content += fmt.Sprintf("\n%d. %s  %d人选择", i+1, //序号
				voteJson.Get(fmt.Sprintf("options.%d.desc", i)).Str(), //描述
				voteJson.Get(fmt.Sprintf("options.%d.cnt", i)).Int())  //选择数
		}
		return content
	}
	log.Debugln("[bilibili] dynamicType:", dynamicType)
	switch dynamicType {
	case "DYNAMIC_TYPE_FORWARD": //转发
		topic := fmt.Sprintf("#%s#\n", dynamic.Get("topic.name").Str()) //话题
		text := fmt.Sprintf("%s", dynamic.Get("desc.text").Str())       //文本
		if !dynamic.Get("topic.name").Nil() {
			content += topic
		}
		content += text + "\n\n" + formatDynamic(json.Get("orig"))
		return head + content
	case "DYNAMIC_TYPE_NONE": //转发的动态已删除
		return json.Get("modules.module_dynamic.major.none.tips").Str() //错误提示: "源动态已被作者删除"
	case "DYNAMIC_TYPE_WORD": //文本
		topic := fmt.Sprintf("#%s#\n", dynamic.Get("topic.name").Str()) //话题
		text := fmt.Sprintf("%s", dynamic.Get("desc.text").Str())       //文本
		if !dynamic.Get("topic.name").Nil() {
			content += topic
		}
		content += text
		if additionalType == "ADDITIONAL_TYPE_VOTE" {
			content += "\n\n" + appendVote(strconv.Itoa(vote.Get("vote_id").Int()))
		}
		if additionalType == "ADDITIONAL_TYPE_RESERVE" {
			title := fmt.Sprintf("%s\n", reserve.Get("title").Str())
			desc1 := fmt.Sprintf("%s    ", reserve.Get("desc1.text").Str()) //"预计xxx发布"
			desc2 := fmt.Sprintf("%s", reserve.Get("desc2.text").Str())     //"xx人预约"/"xx观看"
			content += "\n\n" + title + desc1 + desc2
		}
		return head + content
	case "DYNAMIC_TYPE_DRAW": //图文
		image := "" //图片
		for i := 0; i < len(draw.Get("items").Arr()); i++ {
			image += fmt.Sprintf("[CQ:image,file=%s]", draw.Get(fmt.Sprintf("items.%d.src", i)).Str())
			if i != len(draw.Get("items").Arr())-1 {
				image += "\n"
			}
		}
		topic := fmt.Sprintf("#%s#\n", dynamic.Get("topic.name").Str()) //话题
		text := fmt.Sprintf("%s", dynamic.Get("desc.text").Str())       //文本
		if !dynamic.Get("topic.name").Nil() {
			content += topic
		}
		content += text + image
		if additionalType == "ADDITIONAL_TYPE_VOTE" {
			content += "\n\n" + appendVote(strconv.Itoa(vote.Get("vote_id").Int()))
		}
		if additionalType == "ADDITIONAL_TYPE_RESERVE" {
			title := fmt.Sprintf("%s\n", reserve.Get("title").Str())
			desc1 := fmt.Sprintf("%s    ", reserve.Get("desc1.text").Str()) //"预计xxx发布"
			desc2 := fmt.Sprintf("%s", reserve.Get("desc2.text").Str())     //"xx人预约"/"xx观看"
			content += "\n\n" + title + desc1 + desc2
		}
		return head + content
	case "DYNAMIC_TYPE_AV": //视频
		action := fmt.Sprintf("%s\n\n", author.Get("pub_action").Str())             //"投稿了视频"/"发布了动态视频"
		topic := fmt.Sprintf("#%s#\n", dynamic.Get("topic.name").Str())             //话题
		text := fmt.Sprintf("%s\n", dynamic.Get("desc.text").Str())                 //文本
		cover := fmt.Sprintf("[CQ:image,file=%s]\n", archive.Get("cover").Str())    //封面
		aid := fmt.Sprintf("av%s\n", archive.Get("aid").Str())                      //av号
		title := fmt.Sprintf("%s\n", archive.Get("title").Str())                    //标题
		play := fmt.Sprintf("%s播放  ", archive.Get("stat.play").Str())               //再生
		danmaku := fmt.Sprintf("%s弹幕\n", archive.Get("stat.danmaku").Str())         //弹幕
		link := fmt.Sprintf("www.bilibili.com/video/%s", archive.Get("bvid").Str()) //链接
		content += action
		if !dynamic.Get("topic.name").Nil() {
			content += topic
		}
		if !dynamic.Get("desc.text").Nil() {
			content += text
		}
		content += cover + aid + title
		desc := archive.Get("desc").Str() //简介
		if (desc != "<nil>" && desc != "-") && truncationLength > 0 {
			if len([]rune(desc)) > truncationLength {
				content += fmt.Sprintf("简介：%c......\n", []rune(desc)[0:truncationLength])
			} else {
				content += fmt.Sprintf("简介：%s\n", desc)
			}
		}
		content += play + danmaku + link
		return head + content
	case "DYNAMIC_TYPE_ARTICLE": //文章
		cover := "" //封面组
		for i := 0; i < len(article.Get("image_urls").Arr()); i++ {
			cover += fmt.Sprintf("[CQ:image,file=%s]", article.Get(fmt.Sprintf("image_urls.%d", i)).Str())
			if i == len(article.Get("image_urls").Arr())-1 {
				cover += "\n"
			}
		}
		action := fmt.Sprintf("%s\n\n", author.Get("pub_action").Str())            //"投稿了文章"
		cvid := fmt.Sprintf("\ncv%d\n", article.Get("id").Int())                   //cv号数字
		title := fmt.Sprintf("%s\n", article.Get("title").Str())                   //标题
		label := fmt.Sprintf("%s\n", article.Get("label").Str())                   //xxx阅读
		desc := fmt.Sprintf("简介：%s\n", article.Get("desc").Str())                  //简介
		link := fmt.Sprintf("www.bilibili.com/read/cv%s", article.Get("id").Str()) //链接
		content += action + cover + cvid + title + label + desc + link
		return head + content
	case "DYNAMIC_TYPE_LIVE_RCMD": //直播（动态流拿不到）
		area := fmt.Sprintf("%s - %s\n", //分区
			live.Get("live_play_info.parent_area_name").Str(),
			live.Get("live_play_info.area_name").Str())
		action := fmt.Sprintf("%s\n", author.Get("pub_action").Str())                             //"直播了"
		cover := fmt.Sprintf("[CQ:image,file=%s]\n", live.Get("live_play_info.cover").Str())      //封面
		title := fmt.Sprintf("%s\n", live.Get("live_play_info.title").Str())                      //房间名
		whatched := fmt.Sprintf("%s\n", live.Get("live_play_info.watched_show.text_large").Str()) //xxx人看过
		id := fmt.Sprintf("live.bilibili.com/%d", live.Get("live_play_info.room_id").Int())       //房间号
		content += action + cover + title + area + whatched + id
		return head + content
	case "DYNAMIC_TYPE_COMMON_SQUARE": //应用装扮同步动态
		return head + "这是一条应用装扮同步动态"
	}
	log.Errorln("[bilibili] 未知的动态类型:", dynamicType, id)
	sendMsg2Admin("[bilibili] 未知的动态类型：" + dynamicType + "\n" + id)
	return head + "未知的动态类型"
}

func getArchiveJsonA(aid string) gson.JSON { //.Get("data"))
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/view").
		WithAddQuery("aid", aid).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getArchiveJsonA().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawVideoJsonA", body)
	videoJson := gson.NewFrom(body)
	if videoJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 视频", aid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return videoJson
}

func getArchiveJsonB(bvid string) gson.JSON { //.Get("data"))
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/view").
		WithAddQuery("bvid", bvid).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getArchiveJsonB().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawVideoJsonB", body)
	videoJson := gson.NewFrom(body)
	if videoJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 视频", bvid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return videoJson
}

func formatArchive(videoJson gson.JSON) string {
	var content string
	truncationLength := v.GetInt("parse.settings.descTruncationLength")           //简介截断长度
	pic := fmt.Sprintf("[CQ:image,file=%s]\n", videoJson.Get("pic").Str())        //封面
	aid := fmt.Sprintf("av%d\n", videoJson.Get("aid").Int())                      //av号数字
	title := fmt.Sprintf("%s\n", videoJson.Get("title").Str())                    //标题
	up := fmt.Sprintf("UP：%s\n", videoJson.Get("owner.name").Str())               //up主
	view := fmt.Sprintf("%d播放  ", videoJson.Get("stat.view").Int())               //再生
	danmaku := fmt.Sprintf("%d弹幕  ", videoJson.Get("stat.danmaku").Int())         //弹幕
	reply := fmt.Sprintf("%d回复\n", videoJson.Get("stat.reply").Int())             //回复
	like := fmt.Sprintf("%d点赞  ", videoJson.Get("stat.like").Int())               //点赞
	coin := fmt.Sprintf("%d投币  ", videoJson.Get("stat.coin").Int())               //投币
	favor := fmt.Sprintf("%d收藏\n", videoJson.Get("stat.favorite").Int())          //收藏
	link := fmt.Sprintf("www.bilibili.com/video/%s", videoJson.Get("bvid").Str()) //链接
	content += pic + aid + title + up
	desc := videoJson.Get("desc").Str() //简介
	if (desc != "<nil>" && desc != "-") && truncationLength > 0 {
		if len([]rune(desc)) > truncationLength {
			content += fmt.Sprintf("简介：%c......\n", []rune(desc)[0:truncationLength])
		} else {
			content += fmt.Sprintf("简介：%s\n", desc)
		}
	}
	content += view + danmaku + reply + like + coin + favor + link
	return content
}

func getArticleJson(cvid string) gson.JSON { //.Get("data")
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/article/viewinfo").
		WithAddQuery("id", cvid).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getArticleJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawArticleJson:", body)
	articleJson := gson.NewFrom(body)
	if articleJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 文章", cvid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return articleJson
}

func formatArticle(articleJson gson.JSON, cvid string) string { //文章信息拿不到自己的cv号
	var content string
	image := "" //头图
	for i := 0; i < len(articleJson.Get("image_urls").Arr()); i++ {
		image += "[CQ:image,file=" + articleJson.Get(fmt.Sprintf("image_urls.%d", i)).Str() + "]"
		if i == len(articleJson.Get("image_urls").Arr())-1 {
			image += "\n"
		}
	}
	cv := fmt.Sprintf("cv%d\n", articleJson.Get("id").Int())                       //cv号
	title := fmt.Sprintf("%s\n", articleJson.Get("title").Str())                   //标题
	author := fmt.Sprintf("作者：%s\n", articleJson.Get("author_name").Str())         //作者
	view := fmt.Sprintf("%s阅读  ", articleJson.Get("stats.view").Str())             //阅读
	reply := fmt.Sprintf("%s回复  ", articleJson.Get("stats.reply").Str())           //回复
	share := fmt.Sprintf("%s分享\n", articleJson.Get("stats.share").Str())           //分享
	like := fmt.Sprintf("%s点赞  ", articleJson.Get("stats.like").Str())             //点赞
	coin := fmt.Sprintf("%s投币  ", articleJson.Get("stats.coin").Str())             //投币
	favor := fmt.Sprintf("%s收藏\n", articleJson.Get("stats.favorite").Str())        //收藏
	link := fmt.Sprintf("www.bilibili.com/read/cv%d", articleJson.Get("id").Int()) //链接
	content += image + cv + title + author + view + reply + share + like + coin + favor + link
	return content
}

func getSpaceJson(uid string) gson.JSON { //.Get("data.card")
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/card").
		WithAddQuery("mid", uid).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getSpaceJson().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawSpaceJson:", body)
	spaceJson := gson.NewFrom(body)
	if spaceJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 空间", uid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return spaceJson
}

func formatSpace(spaceJson gson.JSON) string {
	var content string
	pendant := fmt.Sprintf("头像框：%s（%d）\n", //头像框
		spaceJson.Get("pendant.name").Str(),
		spaceJson.Get("pendant.pid").Int())
	face := fmt.Sprintf("[CQ:image,file=%s]\n", spaceJson.Get("face").Str())          //头像
	name := fmt.Sprintf("%s", spaceJson.Get("name").Str())                            //用户名
	level := fmt.Sprintf("（LV%d）\n", spaceJson.Get("level_info.current_level").Int()) //账号等级
	sign := fmt.Sprintf("签名：%s\n", spaceJson.Get("sign").Str())                       //签名
	attention := fmt.Sprintf("%d关注  ", spaceJson.Get("attention").Int())              //关注
	fans := fmt.Sprintf("%d粉丝\n", spaceJson.Get("fans").Int())                        //粉丝
	link := fmt.Sprintf("space.bilibili.com/%s", spaceJson.Get("mid").Str())          //链接
	content += face + name + level
	if pendant != "0" {
		content += pendant
	}
	if sign != "签名：\n" {
		content += sign
	}
	content += attention + fans + link
	return content
}

func getRoomJsonUID(uid string) gson.JSON { //uid获取直播间数据  .Gets("data", strconv.Itoa(uid))
	body := ihttp.New().WithUrl("https://api.live.bilibili.com/room/v1/Room/get_status_info_by_uids").
		WithAddQuery("uids[]", uid).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getRoomJsonUID().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawRoomJson:", body)
	liveJson := gson.NewFrom(body)
	if liveJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 直播间(UID)", uid, "信息获取错误:", body)
	}
	return liveJson
}

func getRoomJsonRoomID(roomID string) gson.JSON { //房间号获取直播间数据（拿不到UP用户名）  .Get("data")
	body := ihttp.New().WithUrl("https://api.live.bilibili.com/room/v1/Room/get_info").
		WithAddQuery("room_id", roomID).WithHeaders(iheaders).Get().
		WithError(func(err error) { log.Errorln("[bilibili] getRoomJsonRoomID().ihttp请求错误:", err) }).ToString()
	log.Traceln("[bilibili] rawRoomJson:", body)
	liveJson := gson.NewFrom(body)
	if liveJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 直播间(RoomID)", roomID, "信息获取错误:", body)
	}
	return liveJson
}

func formatLive(roomJson gson.JSON) string {
	var content string
	area := fmt.Sprintf("%s - %s\n", //分区
		roomJson.Get("area_v2_parent_name").Str(),
		roomJson.Get("area_v2_name").Str())
	cover := fmt.Sprintf("[CQ:image,file=%s]", roomJson.Get("cover_from_user").Str()) //封面
	keyframe := fmt.Sprintf("[CQ:image,file=%s]\n", roomJson.Get("keyframe").Str())   //关键帧
	uname := fmt.Sprintf("%s的直播间", roomJson.Get("uname").Str())                       //主播
	title := fmt.Sprintf("%s\n", roomJson.Get("title").Str())                         //房间名
	link := fmt.Sprintf("live.bilibili.com/%d", roomJson.Get("room_id").Int())        //房间号
	content += cover + keyframe
	switch roomJson.Get("live_status").Int() { //房间状态:   0: "未开播"  1: "直播中 " 2: "轮播中"
	case 0:
		uname += "（未开播）\n"
	case 1:
		uname += "（直播中）\n"
	case 2:
		uname += "（轮播中）\n"
	}
	content += uname + title + area + link
	switch liveStateList[roomJson.Get("room_id").Str()].STATE {
	case streamState.ONLINE:
		content += fmt.Sprintf("机器人缓存的上一次开播时间：\n%s",
			time.Unix(liveStateList[roomJson.Get("room_id").Str()].TIME, 0).Format(timeLayout.M24C))
	case streamState.OFFLINE:
		content += fmt.Sprintf("机器人缓存的上一次下播时间：\n%s",
			time.Unix(liveStateList[roomJson.Get("room_id").Str()].TIME, 0).Format(timeLayout.M24C))
	case streamState.UNKNOWN:
	}
	return content
}
