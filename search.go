package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

const biliSearchRegexp = `[Bb]搜(视频|番剧|影视|直播间|直播|主播|专栏|用户)[\s:：]?(.*)`

var keywordRemove = strings.NewReplacer(`<em class="keyword">`, "", `</em>`, "") //处理html标签

var searchTypes = struct {
	VIDEO     string //视频
	BANGUMI   string //番剧
	FT        string //影视
	LIVE      string //直播(搜不到东西, 转为搜索直播间)
	LIVE_ROOM string //直播间 -> 直播, 直播间
	LIVE_USER string //主播
	ARTICLE   string //专栏
	TOPIC     string //话题(老东西, 搜不出啥)
	USER      string //用户
	PHOTO     string //相簿(搜不到东西)
}{
	VIDEO:     "video",         //视频
	BANGUMI:   "media_bangumi", //番剧
	FT:        "media_ft",      //影视
	LIVE:      "live",          //直播(搜不到东西, 转为搜索直播间)
	LIVE_ROOM: "live_room",     //直播间 -> 直播, 直播间
	LIVE_USER: "live_user",     //主播
	ARTICLE:   "article",       //专栏
	TOPIC:     "topic",         //话题(老东西, 搜不出啥)
	USER:      "bili_user",     //用户
	PHOTO:     "photo",         //相簿(搜不到东西)
}

// 获取搜索并格式化
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
		case "直播", "直播间":
			return searchTypes.LIVE_ROOM
		case "主播":
			return searchTypes.LIVE_USER
		case "专栏":
			return searchTypes.ARTICLE
		case "用户":
			return searchTypes.USER
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
	log.Trace("[search] body: ", g.JSON("", ""))
	if g.Get("code").Int() != 0 {
		return []map[string]any{}
	}
	results := g.Get("data.result").Arr()
	resultCount := len(results)
	forwardNode := appendForwardNode([]map[string]any{}, gocqNodeData{ //标题
		content: []string{fmt.Sprint("快捷搜索", KIND, "(", kind, ") ：\n", keyword, "\n", "共", resultCount, "个结果")},
	})
	switch kind {
	case searchTypes.VIDEO: //视频
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			content: func() []string {
				var content []string
				for _, g := range results {
					pic := g.Get("pic").Str()                            //封面
					aid := g.Get("id").Int()                             //av号数字
					title := keywordRemove.Replace(g.Get("title").Str()) //标题
					up := g.Get("author").Str()                          //up主
					desc := descTrunc(g.Get("description").Str())        //简介
					view := g.Get("play").Int()                          //再生
					danmaku := g.Get("danmaku").Int()                    //弹幕
					review := g.Get("review").Int()                      //评论
					like := g.Get("like").Int()                          //点赞
					favor := g.Get("favorites").Int()                    //收藏
					bvid := g.Get("bvid").Str()                          //bv号
					content = append(content, fmt.Sprintf(
						`[CQ:image,file=https:%s]
av%d
%s
UP：%s%s
%d播放  %d弹幕  %d评论
%d点赞  %d收藏
www.bilibili.com/video/%s`,
						pic,
						aid,
						title,
						up, desc,
						view, danmaku, review,
						like, favor,
						bvid))
				}
				return content
			}(),
		})
	case searchTypes.BANGUMI, searchTypes.FT: //番剧, 影视
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			content: func() (content []string) {
				for _, g := range results {
					cover := g.Get("cover").Str()                                            //封面
					title := keywordRemove.Replace(g.Get("title").Str())                     //汉化名
					titleOrg := keywordRemove.Replace(g.Get("org_title").Str())              //原名
					areas := g.Get("areas").Str()                                            //地区
					styles := g.Get("styles").Str()                                          //类型风格
					season := g.Get("season_type_name").Str()                                //剧集类型（番剧 / 电影 / 纪录片 / 国创 / 电视剧 / 综艺）
					pubt := time.Unix(int64(g.Get("pubtime").Int()), 0).Format("2006-01-02") //开播时间
					index := g.Get("index_show").Str()                                       //更新进度（全xx话 / 更新至第xx话）
					scoreU := g.Get("media_score.user_count").Int()                          //评价人数
					score := g.Get("media_score.score").Num()                                //评分
					cv := g.Get("cv").Str()                                                  //声优
					desc := g.Get("desc").Str()                                              //简介
					badges := func(text gson.JSON, ok bool) (badges string) {                //付费要求
						if ok {
							badges = text.Str() //会员专享、独家、出品
						} else {
							badges = "免费观看"
						}
						return
					}(g.Gets("badges", 0, "text"))
					url := g.Get("url").Str() //链接
					content = append(content, fmt.Sprintf(
						`[CQ:image,file=%s]
%s
%s

%s/%s
%s · %s · %s
%d人评：%.1f分

CV：
%s

简介：%s

%s：%s`,
						cover,
						title,
						titleOrg,
						areas, styles,
						season, pubt, index,
						scoreU, score,
						cv,
						desc,
						badges, url))
				}
				return
			}(),
		})
	case searchTypes.LIVE_ROOM: //直播, 直播间
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			content: func() []string {
				var content []string
				for _, g := range results {
					cover := g.Get("user_cover").Str()                      //封面
					keyframe := g.Get("cover").Str()                        //关键帧
					uname := keywordRemove.Replace(g.Get("uname").Str())    //主播
					title := keywordRemove.Replace(g.Get("title").Str())    //房间名
					cate := keywordRemove.Replace(g.Get("cate_name").Str()) //分区
					tags := g.Get("tags").Str()                             //自定义标签
					liveT := g.Get("live_time").Str()                       //开播时间
					fans := g.Get("attentions").Int()                       //粉丝数
					online := g.Get("online").Int()                         //在线数
					roomid := g.Get("roomid").Int()                         //房间号
					uid := g.Get("uid").Int()                               //uid
					content = append(content, fmt.Sprintf(
						`[CQ:image,file=https:%s][CQ:image,file=https:%s]
%s的直播间%s
分区：%s
%s
开播时间：%s
%d在线  %d粉丝
live.bilibili.com/%d
space.bilibili.com/%d`,
						cover, keyframe,
						uname, title,
						cate,
						tags,
						liveT,
						fans, online,
						roomid,
						uid))
				}
				return content
			}(),
		})
	case searchTypes.LIVE_USER: //主播
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			content: func() []string {
				var content []string
				for _, g := range results {
					live_status := g.Get("live_status").Int()
					uface := g.Get("uface").Str()                        //头像
					uname := keywordRemove.Replace(g.Get("uname").Str()) //主播
					cate := g.Get("cate_name").Str()                     //分区
					tags := g.Get("tags").Str()                          //自定义标签
					liveT := g.Get("live_time").Str()                    //上次直播结束时间
					fans := g.Get("attentions").Int()                    //粉丝数
					roomid := g.Get("roomid").Int()                      //房间号
					uid := g.Get("uid").Int()                            //uid
					if live_status == 0 {                                //未开播直接使用搜索返回的数据
						content = append(content, fmt.Sprintf(
							`[CQ:image,file=https:%s]
%s
%s
%s
上次直播结束于：%s
%d粉丝
live.bilibili.com/%d
space.bilibili.com/%d`,
							uface,
							uname,
							cate,
							tags,
							liveT,
							fans,
							roomid,
							uid))
					} else { //开播则调用getRoomJson和formatLive
						uid := strconv.Itoa(uid)
						roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
						if ok {
							content = append(content, formatLive(roomJson))
						} else { //fallback
							content = append(content, fmt.Sprintf(
								`[CQ:image,file=https:%s]
%s
%s
%s
直播开始于：%s
%d粉丝
live.bilibili.com/%d
space.bilibili.com/%s`,
								uface,
								uname,
								cate,
								tags,
								liveT,
								fans,
								roomid,
								uid))
						}
					}
				}
				return content
			}(),
		})
	case searchTypes.ARTICLE: //专栏
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			content: func() []string {
				var content []string
				for _, g := range results {
					image := g.Get("image_urls").Arr()[0].Str()          //封面
					title := keywordRemove.Replace(g.Get("title").Str()) //标题
					cate := g.Get("category_name").Str()                 //分区
					view := g.Get("view").Int()                          //阅读
					like := g.Get("like").Int()                          //点赞
					reply := g.Get("reply").Int()                        //评论
					desc := g.Get("desc").Str()                          //简介
					cvid := g.Get("id").Int()                            //cv号数字
					mid := g.Get("mid").Int()                            //uid
					content = append(content, fmt.Sprintf(
						`[CQ:image,file=https:%s]
cv%d
%s
%s
%d阅读  %d点赞  %d评论
%s......
www.bilibili.com/read/cv%d
space.bilibili.com/%d`,
						image,
						cvid,
						title,
						cate,
						view, like, reply,
						desc,
						cvid,
						mid))
				}
				return content
			}(),
		})
	case searchTypes.USER: //用户
		forwardNode = appendForwardNode(forwardNode, gocqNodeData{
			content: func() (content []string) {
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
						`[CQ:image,file=https:%s]
%s（LV%d）
space.bilibili.com/%d
%d粉丝  %d投稿%s`,
						upic,
						uname, level,
						mid,
						fans, videos, is_live))
				}
				return
			}(),
		})
	default:
		return []map[string]any{}
	}
	return forwardNode
}

// 哔哩哔哩快捷搜索
func checkSearch(ctx gocqMessage) {
	reg := regexp.MustCompile(biliSearchRegexp).FindAllStringSubmatch(ctx.message, -1)
	if len(reg) > 0 {
		ctx.sendForwardMsg(formatBiliSearch(reg[0][1], reg[0][2]))
	}
}
