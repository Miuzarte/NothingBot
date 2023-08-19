package main

import (
	"fmt"
	"regexp"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
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

func formatBiliSearch(KIND string, keyword string) []map[string]any {
	//KIND = "用户", kind = "bili_user"
	kind := func() string {
		switch KIND {
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
	}()
	log.Debug("[search] 开始搜索: ", KIND, "(", kind, ") ", keyword)
	g, err := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/search/type").
		WithAddQuerys(map[string]any{"search_type": kind, "keyword": keyword}).WithHeaders(iheaders).WithCookie(cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[ihttp] biliSearch().ihttp请求错误: ", err)
	}
	imageSize := func(imageSize int) string { //结果图片压缩尺寸
		if imageSize != 0 {
			return fmt.Sprintf("@%dh", imageSize)
		}
		return ""
	}(v.GetInt("search.settings.imageSize"))
	log.Trace("[search] body: ", g.JSON("", ""))
	if g.Get("code").Int() != 0 {
		return []map[string]any{}
	}
	results := g.Get("data.result").Arr()
	resultCount := len(results)
	forwardNode := appendForwardNode([]map[string]any{}, gocqNodeData{ //标题
		content: []string{fmt.Sprint("快捷搜索", KIND, "(", kind, ") ：\n", keyword, "\n共", resultCount, "个结果")},
	})
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
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			content: func() []string {
				var content []string
				for _, g := range results {
					upic := g.Get("upic").String()   //头像
					uname := g.Get("uname").String() //昵称
					level := g.Get("level").Int()    //等级
					mid := g.Get("mid").Int()        //uid
					fans := g.Get("fans").Int()      //粉丝数
					videos := g.Get("videos").Int()  //视频数
					is_live := func() string {       //直播状态
						if g.Get("is_live").Int() != 0 {
							return fmt.Sprint("\n直播中：live.bilibili.com/", g.Get("room_id").Int())
						}
						return ""
					}()
					content = append(content, fmt.Sprintf(
						`[CQ:image,file=https:%s%s]
%s（LV%d）
space.bilibili.com/%d
%d粉丝  %d投稿%s`,
						upic, imageSize,
						uname, level,
						mid,
						fans, videos, is_live))
				}
				return content
			}(),
		})
	case searchTypes.PHOTO:
	default:
		return []map[string]any{}
	}
	return forwardNode
}

func checkSearch(ctx gocqMessage) {
	reg := regexp.MustCompile(biliSearchRegexp).FindAllStringSubmatch(ctx.message, -1)
	if len(reg) != 0 {
		sendForwardMsgCTX(ctx, formatBiliSearch(reg[0][1], reg[0][2]))
	}
}
