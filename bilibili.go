package main

import (
	"NothinBot/EasyBot"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/moxcomic/bcutasr"
	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

type BiliApiResp struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Ttl     int            `json:"ttl"`
	Data    map[string]any `json:"data"`
}

type liveInfo struct {
	live   *danmaku
	uid    int
	roomid int
	state  int
	time   int64
}

type push struct {
	userID  []int
	groupID []int
}

type parseHistory struct {
	parse string
	time  int64
}

var (
	biliLinkRegexp = struct {
		SHORT     string
		DYNAMIC   string
		ARCHIVEav string
		ARCHIVEbv string
		ARTICLE   string
		MUSIC     string
		SPACE     string
		LIVE      string
	}{
		SHORT:     `((让岁己)?(总结一下\s?)|我要看\s?)?.*(b23|acg)\.tv\\?/(BV[1-9A-HJ-NP-Za-km-z]{10}|av[0-9]{1,10}|[0-9A-Za-z]{7})`, //暂时应该只有7位  也有可能是av/bv号
		DYNAMIC:   `(让岁己)?(总结一下\s?)?.*(t.bilibili.com|dynamic|opus)\\?/([0-9]{18,19})`,                                     //应该不会有17位的，可能要有19位
		ARCHIVEav: `((让岁己)?(总结一下\s?)|我要看\s?)?.*video\\?/av([0-9]{1,10})`,                                                   //9位 预留10
		ARCHIVEbv: `((让岁己)?(总结一下\s?)|我要看\s?)?.*video\\?/(BV[1-9A-HJ-NP-Za-km-z]{10})`,                                      //恒定BV + 10位base58
		ARTICLE:   `(让岁己)?(总结一下\s?)?.*(read\\?/cv|read\\?/mobile\\?/)([0-9]{1,9})`,                                         //8位 预留9
		MUSIC:     `(让岁己)?(总结一下\s?)?.*audio\\?/au([0-9]{1,10})`,                                                            //
		SPACE:     `(让岁己)?(总结一下\s?)?.*space\.bilibili\.com\\?/([0-9]{1,16})`,                                               //新uid 16位
		LIVE:      `(让岁己)?(总结一下\s?)?.*live\.bilibili\.com\\?/([0-9]{1,9})`,                                                 //8位 预留9
	}

	liveState = struct {
		UNKNOWN int
		OFFLINE int
		ONLINE  int
		ROTATE  int
	}{
		UNKNOWN: -1,
		OFFLINE: 0,
		ONLINE:  1,
		ROTATE:  2,
	}

	rmTitle = strings.NewReplacer("概述", "", "要点", "", "由", "", "总结：", "", "总结（原文长度超过1500字符，输入经过去尾）：", "",
		"ChatGLM2-6B", "", "ERNIE_Bot", "", "ERNIE_Bot_turbo", "", "BLOOMZ_7B", "", "Llama_2_7b", "", "Llama_2_13b", "", "Llama_2_70b", "")

	// cookie               = ""
	cookieUid            = 0
	cookieBuvid          = "91F87C44-8B65-64C4-296C-B102F459941CF05635infoc" //扫码拿不到 先写着
	cookieValidity       = false
	tempDir              = "./bilibili_temp/"
	summaryBackend       = ""
	dynamicCheckDuration time.Duration
	dynamicHistrory      = make(map[string]string)
	pushWait             sync.WaitGroup
	liveList             = make(map[int]liveInfo)         // roomid : liveInfo
	archiveVideoTable    = make(map[int]*archiveVideo)    //av:
	archiveAudioTable    = make(map[int]*archiveAudio)    //av:
	archiveSubtitleTable = make(map[int]*archiveSubtitle) //av:
	articleTextTable     = make(map[int]*articleText)     //cv:
	groupParseHistory    = make(map[int]parseHistory)     //group:
)

var everyBiliLinkRegexp = func() (everyBiliLinkRegexp string) {
	structValue := reflect.ValueOf(biliLinkRegexp)
	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		if everyBiliLinkRegexp != "" {
			everyBiliLinkRegexp += "|"
		}
		everyBiliLinkRegexp += field.Interface().(string)
	}
	return
}()

const standardLength = len("BV1vh4y1U71j")

// bv转av
func bv2av(bv string) (av int) {
	if length := len(bv); length != standardLength {
		log.Warn("[bv2av] 输入了错误的bv号: ", bv, " (len: ", length, ")")
		return 0
	}
	table := "fZodR9XQDSUm21yCkr6zBqiveYah8bt4xsWpHnJE7jL5VG3guMTKNPAwcF"
	tr := make(map[byte]int)
	for i := 0; i < 58; i++ {
		tr[table[i]] = i
	}
	s := []int{11, 10, 3, 8, 4, 6}
	xor := 177451812
	add := 8728348608
	r := 0
	for i := 0; i < 6; i++ {
		r += tr[bv[s[i]]] * int(math.Pow(58, float64(i)))
	}
	av = (r - add) ^ xor
	log.Debug("[Bilibili] ", bv, " 转换到 av", av)
	return
}

func CallBiliApi(url string, querys map[string]any) (resp *BiliApiResp, headers map[string]string, err error) {
	resp = &BiliApiResp{}
	i := ihttp.New().WithUrl(url).
		WithHeaders(iheaders).WithAddQuerys(querys).Get()
	body, err := i.ToBytes()
	if err != nil {
		log.Error("[Bilibili] CallBiliApi() ihttp ToBytes error: ", err)
		return
	}
	header, err := i.ToHeader()
	if err != nil {
		log.Error("[Bilibili] CallBiliApi() ihttp ToHeader error: ", err)
		return
	}
	err = json.Unmarshal(body, resp)
	if err != nil {
		log.Error("[Bilibili] CallBiliApi() unmarshal error: ", err,
			"\n    data: ", string(body),
			"\n    using gson: ", gson.New(body).JSON("", ""))
		return
	}
	headers = make(map[string]string)
	for k, v := range header {
		headers[k] = strings.Join(v, " ")
	}
	return
}

// 获取动态数据.Get("data.item")
func getDynamicJson(dynamicID string) gson.JSON {
	dynamicJson, err := ihttp.New().WithUrl("https://api.bilibili.com/x/polymer/web-dynamic/v1/detail").
		WithAddQuery("id", dynamicID).WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getDynamicJson().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawDynamicJson: ", dynamicJson.JSON("", ""))
	if dynamicJson.Get("code").Int() != 0 {
		log.Error("[parse] 动态 ", dynamicID, " 信息获取错误: ", dynamicJson.JSON("", ""))
	}
	return dynamicJson
}

// 获取投票数据.Get("data.info")
func getVoteJson(voteid int) gson.JSON {
	voteJson, err := ihttp.New().WithUrl("https://api.vc.bilibili.com/vote_svr/v1/vote_svr/vote_info").
		WithAddQuerys(map[string]any{"vote_id": voteid}).WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getVoteJson().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawVoteJson: ", voteJson.JSON("", ""))
	if voteJson.Get("code").Int() != 0 {
		log.Error("[parse] 投票 ", voteid, " 信息获取错误: ", voteJson.JSON("", ""))
	}
	return voteJson
}

// 格式化动态, 主动态.Get("data.item"), 转发原动态.Get("data.item.orig")
func formatDynamic(g gson.JSON) string {
	dynamic := g.Get("modules.module_dynamic")                //动态主体
	id := g.Get("id_str").Str()                               //动态id
	uid := g.Get("modules.module_author.mid").Int()           //发布者uid
	name := g.Get("modules.module_author.name").Str()         //发布者用户名
	action := g.Get("modules.module_author.pub_action").Str() //"投稿了视频"/"发布了动态视频"/"投稿了文章"/"直播了"
	topic := func(exist bool) (topic string) {                //话题
		if exist {
			topic = "\n#" + dynamic.Get("topic.name").Str() + "#"
		}
		return
	}(!dynamic.Get("topic.name").Nil())
	addition := func(additionalType string) (addtion string) { //子项内容
		switch additionalType {
		case "ADDITIONAL_TYPE_RESERVE": //预约
			reserveJson := dynamic.Get("additional.reserve")
			addtion = fmt.Sprintf("\n%s\n%s\n%s",
				reserveJson.Get("title").Str(),
				reserveJson.Get("desc1.text").Str(), //"预计xxx发布"
				reserveJson.Get("desc2.text").Str()) //"xx人预约"/"xx观看"
		case "ADDITIONAL_TYPE_VOTE": //投票
			voteJson := getVoteJson(dynamic.Get("additional.vote.vote_id").Int()).Get("data.info")
			name := voteJson.Get("name").Str()            //发起者
			title := voteJson.Get("title").Str()          //标题
			desc := descTrunc(voteJson.Get("desc").Str()) //简介
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
			}(int64(voteJson.Get("starttime").Int()), int64(voteJson.Get("endtime").Int()))
			c_cnt := voteJson.Get("choice_cnt").Int()             //最大选择数
			cnt := voteJson.Get("cnt").Int()                      //参与数
			option := func(options []gson.JSON) (option string) { //选项
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
						//cookie失效时拿不到选择数
					}
				}
				return
			}(voteJson.Get("options").Arr())
			addtion = fmt.Sprintf(`
%s发起的投票：%s%s
%s  -  %s
最多选%d项  %d人参与%s`,
				name, title, desc,
				startTime, endTime,
				c_cnt, cnt, option)
		case "ADDITIONAL_TYPE_UGC": //评论同时转发
			url := dynamic.Get("additional.ugc.jump_url").Str()
			id, kind, _, _, _ := extractBiliLink(url)
			addtion = "\n\n转发的视频：\n" + parseAndFormatBiliLink(nil, id, kind, false, false, false)
		}
		return
	}(dynamic.Get("additional.type").Str())

	dynamicType := g.Get("type").Str() //动态类型
	log.Debug("[bilibili] 动态类型: ", dynamicType)
	switch dynamicType {
	case "DYNAMIC_TYPE_FORWARD": //转发
		text := dynamic.Get("desc.text").Str() //正文
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
		return dynamic.Get("major.none.tips").Str() //错误提示: "源动态已被作者删除"
	case "DYNAMIC_TYPE_WORD": //纯文字
		text := dynamic.Get("desc.text").Str() //正文
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s
%s%s`,
			id,
			name, topic,
			text, addition)
	case "DYNAMIC_TYPE_DRAW": //图文
		draw := dynamic.Get("major.draw")
		images := func(items []gson.JSON) (images string) { //图片
			for _, item := range items {
				images += fmt.Sprint("[CQ:image,file=", item.Get("src").Str(), "]")
			}
			return
		}(draw.Get("items").Arr())
		text := dynamic.Get("desc.text").Str() //正文
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s
%s
%s%s`,
			id,
			name, topic,
			text,
			images, addition)
	case "DYNAMIC_TYPE_AV": //视频
		archive := dynamic.Get("major.archive")
		text := func(exist bool, text string) string { //正文
			if text == archive.Get("desc").Str() { //如果正文和简介相同, 不显示正文
				return ""
			}
			if exist {
				return "\n" + text
			}
			return ""
		}(!dynamic.Get("desc.text").Nil(), dynamic.Get("desc.text").Str())
		aid, _ := strconv.Atoi(archive.Get("aid").Str()) //av号数字
		content := func() (content string) {
			g, h := getArchiveJson(aid)
			if g.Get("code").Int() != 0 {
				return fmt.Sprintf("[NothingBot] [ERROR] [parse] 视频av%s信息获取错误: code%d", id, g.Get("code").Int())
			}
			content = formatArchive(g.Get("data"), h.Get("data"))
			return
		}()
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s%s%s

%s`,
			id,
			name, action, topic, text,
			content)
	case "DYNAMIC_TYPE_ARTICLE": //文章
		article := dynamic.Get("major.article")
		cvid := article.Get("id").Int() //cv号数字
		content := func() (content string) {
			g := getArticleJson(cvid)
			if g.Get("code").Int() != 0 {
				return fmt.Sprintf("[NothingBot] [ERROR] [parse] 专栏cv%s信息获取错误: code%d", id, g.Get("code").Int())
			}
			return formatArticle(g.Get("data"), cvid)
		}()
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s%s

%s`,
			id,
			name, action, topic,
			content)
	case "DYNAMIC_TYPE_MUSIC":
		music := dynamic.Get("major.music")
		sid := music.Get("id").Int()
		content := func() (content string) {
			g, h, i := getMusicJson(sid)
			if g.Get("code").Int() != 0 || h.Get("code").Int() != 0 || i.Get("code").Int() != 0 {
				return fmt.Sprintf("[NothingBot] [ERROR] [parse] 专栏cv%s信息获取错误: code%d", id, g.Get("code").Int())
			}
			return formatMusic(g.Get("data"), h.Get("data"), i.Get("data"))
		}()
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s%s

%s`,
			id,
			name, action, topic,
			content)
	case "DYNAMIC_TYPE_LIVE_RCMD": //直播（动态流拿不到更新）
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s

%s`,
			id,
			name, action,
			formatLive(getRoomJsonUID(uid)))
	case "DYNAMIC_TYPE_COMMON_SQUARE": //应用装扮同步动态
		log.Info("[bilibili] 应用装扮同步动态: ", dynamic.JSON("", ""))
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：%s%s%s

这是一条应用装扮同步动态：%s`,
			id,
			name, action, topic, addition,
			dynamicType)
	default:
		log.Error("[bilibili] 未知的动态类型: ", dynamicType, id)
		bot.Log2SU.Error(fmt.Sprint("[bilibili] 未知的动态类型：", dynamicType, " (", id, ")"))
		return fmt.Sprintf(
			`t.bilibili.com/%s
%s：

未知的动态类型：%s`,
			id,
			name,
			dynamicType)
	}
}

// 获取官方AI总结
func getArchiveSummary(aid int) (summary string, err error) {
	cid := getCid(aid)
	signedUrl := SignURL(fmt.Sprintf("https://api.bilibili.com/x/web-interface/view/conclusion/get?aid=%d&cid=%d", aid, cid))
	videoSummary, err := ihttp.New().WithUrl(signedUrl).WithHeaders(iheaders).Get().ToGson()
	if err != nil {
		return
	}
	// 大总结
	summary = videoSummary.Get("data.model_result.summary").Str()
	// 大纲
	outlines := videoSummary.Get("data.model_result.outline").Arr()
	if summary == "" && len(outlines) == 0 {
		return "", nil
	}
	for _, outline := range outlines {
		summary += "\n● " + outline.Get("title").Str()
		// 小节
		for _, partOutline := range outline.Get("part_outline").Arr() {
			timestamp := partOutline.Get("timestamp").Int()
			content := partOutline.Get("content").Str()
			summary += fmt.Sprintf("\n[%s] %s",
				formatTimeSimple(int64(timestamp)), content,
			)
		}
	}
	return
}

// av号获取视频数据.Get("data"))
func getArchiveJson(aid int) (archiveJson gson.JSON, stateJson gson.JSON) {
	archiveJson, err := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/view").
		WithAddQuerys(map[string]any{"aid": aid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getArchiveJsonA().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawArchiveJsonA: ", archiveJson.JSON("", ""))
	if archiveJson.Get("code").Int() != 0 {
		log.Error("[parse] 视频 ", aid, " 信息获取错误: ", archiveJson.JSON("", ""))
	}
	cid := archiveJson.Get("data.cid").Int()
	stateJson, err = ihttp.New().WithUrl("https://api.bilibili.com/x/player/online/total").
		WithAddQuerys(map[string]any{"aid": aid, "cid": cid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getArchiveJsonA().statJson.ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawArchiveJsonA.state: ", archiveJson.JSON("", ""))
	if stateJson.Get("code").Int() != 0 {
		log.Error("[parse] 视频 ", aid, " 在线人数状态获取错误: ", stateJson.JSON("", ""))
	}
	return
}

// bv号获取视频数据.Get("data"))
func getArchiveJsonB(bvid string) (archiveJson gson.JSON, stateJson gson.JSON) {
	archiveJson, err := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/view").
		WithAddQuery("bvid", bvid).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getArchiveJsonB().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawVideoJsonB: ", archiveJson.JSON("", ""))
	if archiveJson.Get("code").Int() != 0 {
		log.Error("[parse] 视频 ", bvid, " 信息获取错误: ", archiveJson.JSON("", ""))
	}
	cid := archiveJson.Get("data.cid").Int()
	stateJson, err = ihttp.New().WithUrl("https://api.bilibili.com/x/player/online/total").
		WithAddQuerys(map[string]any{"bvid": bvid, "cid": cid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getArchiveJsonB().statJson.ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] getArchiveJsonB.statJson: ", archiveJson.JSON("", ""))
	if stateJson.Get("code").Int() != 0 {
		log.Error("[parse] 视频 ", bvid, " 在线人数状态获取错误: ", stateJson.JSON("", ""))
	}
	return
}

// 读取缓存
func initCache() {
	_ = checkDir(tempDir)
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Error("访问路径 ", path, " 时发生错误: ", err.Error())
			return err
		}
		if info.IsDir() {
			return nil
		}
		fileDataRaw, err := os.ReadFile(path)
		if err != nil {
			log.Error("[bilibili] read cache err: ", err.Error())
		}
		g := gson.New(fileDataRaw)
		fileData := []byte(g.JSON("", ""))
		switch info.Name()[:2] { //文件名前两个字母
		case "av":
			as := &archiveSubtitle{}
			err = json.Unmarshal(fileData, as)
			if err != nil {
				log.Error("[NothingBot] 反序列化出错(json.Unmarshal(fileData, as)), err: ", err,
					"\n    respByte: ", string(fileData),
					"\n    Unmarshal by gson: ", gson.New(fileData).JSON("", ""))
				break
			}
			as.marshal()
			archiveSubtitleTable[as.Aid] = as
			if !as.IsNative {
				aacPath := fmt.Sprint("av", as.Aid, "_c", as.Cid, ".aac")
				_, err := os.Stat(aacPath)
				if err == nil { //存在已下载的音频文件
					archiveAudioTable[as.Aid] = &archiveAudio{
						aid:       as.Aid,
						cid:       as.Cid,
						localPath: aacPath,
					}
				}
			}
		case "cv":
			at := &articleText{}
			err = json.Unmarshal(fileData, at)
			if err != nil {
				log.Error("[NothingBot] 反序列化出错(json.Unmarshal(fileData, at)), err: ", err,
					"\n    respByte: ", string(fileData),
					"\n    Unmarshal by gson: ", gson.New(fileData).JSON("", ""))
				break
			}
			at.marshal()
			articleTextTable[at.Cvid] = at
		}
		return nil
	})
	if err != nil {
		log.Error("遍历缓存时发生错误: ", err.Error())
	}
}

var videoCodec = struct {
	avc  int
	hevc int
	av1  int
}{
	avc:  7,
	hevc: 12,
	av1:  13,
}

var videoQual = struct {
	js240      int
	lc360      int
	qx480      int
	gq720      int
	gzl720p60  int
	gq1080     int
	gml1080    int
	gzl1080p60 int
	cq4K       int
	zcsHDR     int
	dolby      int
	cgq8K      int
}{
	js240:      6,
	lc360:      16,
	qx480:      32,
	gq720:      64,
	gzl720p60:  74,
	gq1080:     80,
	gml1080:    112,
	gzl1080p60: 116,
	cq4K:       120,
	zcsHDR:     125,
	dolby:      126,
	cgq8K:      127,
}

type archiveVideo struct {
	aid      int
	cid      int
	hasAudio bool //是否带音频
	path     string
}

// 获取视频流(mp4)
func getVideoMp4(aid int, qual int) *archiveVideo {
	if cacheVi, has := archiveVideoTable[aid]; has {
		return cacheVi
	}
	checkDir(tempDir)
	cid := getCid(aid)
	url := getVideoUrlMp4(aid, cid, qual)
	path := ""

	if lor := gocqIsLocalOrRemote(); lor == "local" { //gocq在本地时通过bot下载
		fileName := fmt.Sprint("av", aid, "_c", cid, "_qn", qual, ".mp4")
		path := tempDir + fileName
		videoByte, err := ihttp.New().WithUrl(url).
			WithHeaders(iheaders).
			Get().ToBytes()
		if err != nil {
			log.Error("[bilibili] 视频(mp4)下载失败 err: ", err)
			return nil
		}
		err = os.WriteFile(path, videoByte, 0664)
		if err != nil {
			log.Error("[bilibili] 视频(mp4)写入本地失败 err: ", err)
		}
		log.Debug("[bilibili] local path: ", path, "  len(videoByte): ", len(videoByte))
	} else if lor == "remote" { //否则调用远程下载
		p, err := bot.DownloadFile(url, 1, iheaders)
		path = p
		if err != nil {
			log.Error("[bilibili] 远程视频(mp4)下载失败 err: ", err)
			return nil
		} else {
			log.Debug("[bilibili] remote path: ", path)
		}
	}

	return &archiveVideo{
		aid:      aid,
		cid:      cid,
		hasAudio: true,
		path:     path,
	}
}

type videoUrls map[int]map[int]string //qual:codec:

// 获取视频流(dash)链接
func getVideoUrlDash(aid int, cid int) (urls videoUrls) {
	g, err := ihttp.New().WithUrl(`https://api.bilibili.com/x/player/playurl`).
		WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		WithAddQuerys(map[string]any{
			"avid":  aid,
			"cid":   cid,
			"fnval": 16, //dash
		}).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] 获取视频流(dash)链接失败 err: ", err)
		return
	}
	if g.Get("code").Int() != 0 {
		log.Error("[bilibili] 获取视频流(dash)链接失败 g: ", g.JSON("", ""))
		return
	}
	urls = make(videoUrls)
	for _, h := range g.Get("data.dash.video").Arr() {
		qualId := h.Get("id").Int()
		urls[qualId] = make(map[int]string)
		codecId := h.Get("codecid").Int()
		baseUrl := h.Get("baseUrl").Str()
		urls[qualId][codecId] = baseUrl
	}
	return
}

// 获取视频流(mp4)链接, avc only
func getVideoUrlMp4(aid int, cid int, qual int) (url string) {
	g, err := ihttp.New().WithUrl(`https://api.bilibili.com/x/player/playurl`).
		WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		WithAddQuerys(map[string]any{
			"avid":  aid,
			"cid":   cid,
			"qn":    qual,
			"fnval": 1, //mp4
		}).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] 获取视频流(mp4)链接失败 err: ", err)
		return
	}
	if g.Get("code").Int() != 0 {
		log.Error("[bilibili] 获取视频流(mp4)链接失败 g: ", g.JSON("", ""))
		return
	}
	url = g.Get("data.durl").Arr()[0].Get("url").Str()
	if len(url) < 16 {
		log.Error("[bilibili] 获取视频流(mp4)链接失败 url: ", url, " g: ", g.JSON("", ""))
		return ""
	}
	return
}

var audioQual = struct {
	low   int //64k
	mid   int //132k
	high  int //192k
	dolby int
	HiRes int
}{
	low:   30216,
	mid:   30232,
	high:  30280,
	dolby: 30250,
	HiRes: 30251,
}

type archiveAudio struct {
	aid       int
	cid       int
	localPath string
}

// 获取音频流
func getAudio(aid int, cid int) *archiveAudio {
	if cacheAu, has := archiveAudioTable[aid]; has {
		return cacheAu
	}
	checkDir(tempDir)
	url := getAudioUrl(aid, cid).high()
	fileName := fmt.Sprint("av", aid, "_c", cid, ".aac")
	localPath := tempDir + fileName
	audioByte, err := ihttp.New().WithUrl(url).
		WithHeaders(iheaders).
		Get().ToBytes()
	if err != nil {
		log.Error("[bilibili] 音频下载失败 err: ", err)
		return nil
	} else {
		log.Debug("[bilibili] len(audioByte): ", len(audioByte))
	}
	os.WriteFile(localPath, audioByte, 0664)
	return &archiveAudio{
		aid:       aid,
		cid:       cid,
		localPath: localPath,
	}
}

type audioUrls map[int]string

// 获取音频流链接
func getAudioUrl(aid int, cid int) (urls audioUrls) {
	g, err := ihttp.New().WithUrl(`https://api.bilibili.com/x/player/playurl`).
		WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		WithAddQuerys(map[string]any{
			"avid":  aid,
			"cid":   cid,
			"fnval": 16, //dash
		}).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] 获取音频流链接失败 err: ", err)
		return
	}
	if g.Get("code").Int() != 0 {
		log.Error("[bilibili] 获取音频流链接失败 g: ", g.JSON("", ""))
		return
	}
	urls = make(audioUrls)
	for _, h := range g.Get("data.dash.audio").Arr() {
		qualId := h.Get("id").Int()
		baseUrl := h.Get("baseUrl").Str()
		urls[qualId] = baseUrl
	}
	return
}

// 尽量获取192k
func (a audioUrls) high() (bestUrl string) {
	urlHigh, hasHigh := a[audioQual.high]
	urlMid, hasMid := a[audioQual.mid]
	urlLow, hasLow := a[audioQual.low]
	switch {
	case hasHigh:
		bestUrl = urlHigh
	case hasMid:
		bestUrl = urlMid
	case hasLow:
		bestUrl = urlLow
	default:
		for _, j := range a {
			bestUrl = j
			break
		}
	}
	log.Trace("[bilibili] bestUrl: ", bestUrl)
	return
}

type archiveSubtitle struct {
	Aid      int    `json:"aid"`
	Cid      int    `json:"cid"`
	Up       string `json:"up"`
	Title    string `json:"title"`
	Result   string `json:"result"` //gson.JSON.JSON("","")
	seq      string //不存本地
	IsNative bool   `json:"is_native"` //真为原生字幕，假为转录字幕
}

// 获取视频原生字幕/缓存字幕, 传标题进来省得再请求一遍
func getSubtitle(aid int, up string, title string) *archiveSubtitle {
	if cacheAS, has := archiveSubtitleTable[aid]; has && cacheAS != nil {
		log.Info("[bilibili] 调用缓存: av", aid)
		return cacheAS
	}
	cid := getCid(aid)
	if cid == 0 {
		log.Error("[bilibili] cid == 0")
		return nil
	}
	subtitleUrl := getSubtitleUrl(aid, cid)
	log.Trace("[bilibili] subtitleUrl: ", subtitleUrl)
	if subtitleUrl == "" {
		log.Error("[bilibili] subtitleUrl == \"\"")
		return nil
	}
	result, err := ihttp.New().WithUrl("https:" + subtitleUrl).
		WithHeaders(iheaders).
		Get().ToString()
	if err != nil {
		log.Error("[bilibili] ihttp err: ", err)
		return nil
	}
	as := &archiveSubtitle{
		Aid:      aid,
		Cid:      cid,
		Up:       up,
		Title:    title,
		Result:   result,
		IsNative: true,
	}
	checkDir(tempDir)
	asByte, err := json.Marshal(as)
	if err != nil {
		log.Error("[bilibili] Cache Marshal err: ", err.Error())
	}
	localPath := fmt.Sprint(tempDir, "av", aid, "_c", cid, ".json")
	os.WriteFile(localPath, asByte, 0644) //缓存
	as.nativeMarshal()
	return as
}

// 调用必剪转录视频字幕
func bcutSubtitle(aid int, up string, title string) *archiveSubtitle {
	checkDir(tempDir)
	cid := getCid(aid)
	if cid == 0 {
		log.Error("[bilibili] cid == 0")
		return nil
	}
	audio := getAudio(aid, cid)
	resp, err := bcutasr.New().Parse(audio.localPath)
	if err != nil {
		panic(err)
	}
	log.Debug("[bilibili] bcutASR code: ", resp.GetInt("code"))
	result := resp.GetString("data.result")
	as := &archiveSubtitle{
		Aid:      aid,
		Cid:      cid,
		Up:       up,
		Title:    title,
		Result:   result,
		IsNative: false,
	}
	checkDir(tempDir)
	asByte, err := json.Marshal(as)
	if err != nil {
		log.Error("[bilibili] Cache Marshal err: ", err.Error())
	}
	localPath := fmt.Sprint(tempDir, "av", aid, "_c", cid, ".json")
	os.WriteFile(localPath, asByte, 0644) //缓存
	as.bcutMarshal()
	return as
}

// 序列化
func (as *archiveSubtitle) marshal() *archiveSubtitle {
	if as.IsNative {
		as.nativeMarshal()
	} else {
		as.bcutMarshal()
	}
	return as
}

// 原生字幕序列化
func (as *archiveSubtitle) nativeMarshal() *archiveSubtitle {
	as.seq = func() (seq string) {
		resultJson := gson.NewFrom(as.Result)
		for _, body := range resultJson.Get("body").Arr() {
			if seq != "" {
				seq += "\n"
			}
			seq += body.Get("content").Str()
		}
		return
	}()
	return as
}

// 必剪转录文本序列化
func (as *archiveSubtitle) bcutMarshal() *archiveSubtitle {
	as.seq = func() (seq string) {
		resultJson := gson.NewFrom(as.Result)
		for _, sent := range resultJson.Get("utterances").Arr() {
			if seq != "" {
				seq += "\n"
			}
			seq += sent.Get("transcript").Str()
		}
		return
	}()
	return as
}

// 获取p1的cid
func getCid(aid int) (cid int) {
	pagelist, err := ihttp.New().WithUrl("https://api.bilibili.com/x/player/pagelist").
		WithAddQuerys(map[string]any{"aid": aid}).WithHeaders(iheaders).
		Get().ToGson()
	if err == nil && pagelist.Get("code").Int() == 0 {
		cid = pagelist.Get("data.0.cid").Int()
	} else {
		log.Error("[bilibili] cid获取错误  err: ", err, " code: ", pagelist.Get("code").Int())
	}
	return
}

// 获取视频字幕链接
func getSubtitleUrl(aid int, cid int) (url string) {
	player, err := ihttp.New().WithUrl("https://api.bilibili.com/x/player/v2").
		WithAddQuerys(map[string]any{"aid": aid, "cid": cid}).WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		Get().ToGson()
	if err == nil || player.Get("code").Int() == 0 {
		subtitles := player.Get("data.subtitle.subtitles").Arr()
		if len(subtitles) == 0 { //没有字幕
			log.Trace("[bilibili] len(subtitles) == 0")
			log.Trace("[bilibili] player: ", player.JSON("", ""))
			return
		}
		subtitlesMap := make(map[string]string) // "lan":"subtitle_url"
		for _, subtitle := range subtitles {
			lan := subtitle.Get("lan").Str()
			url := subtitle.Get("subtitle_url").Str()
			subtitlesMap[lan] = url
		}
		urlZHCN, hasZHCN := subtitlesMap["zh-CN"]
		urlAIZH, hasAIZH := subtitlesMap["ai-zh"]
		if hasZHCN {
			url = urlZHCN
		} else if hasAIZH {
			url = urlAIZH
		} else { //都没有直接取第一个
			url = subtitles[0].Get("subtitle_url").Str()
		}
	} else {
		log.Error("[bilibili] 字幕获取错误  err: ", err, " code: ", player.Get("code").Int())
	}
	return
}

// 简介截断
func descTrunc(desc string) string {
	truncationLength := v.GetInt("parse.settings.descTruncationLength")
	if (desc != "" && desc != "-") && truncationLength > 0 {
		if len([]rune(desc)) > truncationLength {
			return fmt.Sprint("\n简介：", string([]rune(desc)[0:truncationLength]), "......")
		} else {
			return fmt.Sprint("\n简介：", desc)
		}
	}
	return ""
}

const (
	_ = iota
	numA
	numB = numA * 10
	numC = numB * 10
	numD = numC * 10
	numE = numD * 10
	numF = numE * 10
	numG = numF * 10
	numH = numG * 10
	numI = numH * 10
)

func formatView[T int | float32 | float64](num T) string {
	switch {
	case num > numI: // 1亿
		return fmt.Sprintf("%.3g亿", float64(num)/numI)
	case num > numE: // 1万
		return fmt.Sprintf("%.3g万", float64(num)/numE)
	default:
		return strconv.Itoa(int(num))
	}
}

// 格式化视频.Get("data"))
func formatArchive(g gson.JSON, h gson.JSON) string {
	pic := g.Get("pic").Str()              //封面
	aid := g.Get("aid").Int()              //av号数字
	title := g.Get("title").Str()          //标题
	up := g.Get("owner.name").Str()        //up主
	desc := descTrunc(g.Get("desc").Str()) //简介
	view := g.Get("stat.view").Int()       //再生
	danmaku := g.Get("stat.danmaku").Int() //弹幕
	reply := g.Get("stat.reply").Int()     //评论
	like := g.Get("stat.like").Int()       //点赞
	coin := g.Get("stat.coin").Int()       //投币
	favor := g.Get("stat.favorite").Int()  //收藏
	bvid := g.Get("bvid").Str()            //bv号
	total := func() string {               //在线人数
		if !h.Get("total").Nil() {
			total := h.Get("total").Str()
			if total == "1" {
				return ""
			} else {
				return fmt.Sprintf("\n%s人在看", total)
			}
		} else {
			return ""
		}
	}()
	return fmt.Sprintf(
		`[CQ:image,file=%s]
av%d
%s
UP：%s%s%s
%s播放  %s弹幕  %s评论
%s点赞  %s投币  %s收藏
www.bilibili.com/video/%s`,
		pic,
		aid,
		title,
		up, desc, total,
		formatView(view), formatView(danmaku), formatView(reply),
		formatView(like), formatView(coin), formatView(favor),
		bvid)
}

// 获取文章数据.Get("data")
func getArticleJson(cvid int) gson.JSON {
	articleJson, err := ihttp.New().WithUrl("https://api.bilibili.com/x/article/viewinfo").
		WithAddQuerys(map[string]any{"id": cvid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getArticleJson().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawArticleJson: ", articleJson.JSON("", ""))
	if articleJson.Get("code").Int() != 0 {
		log.Error("[parse] 文章 ", cvid, " 信息获取错误: ", articleJson.JSON("", ""))
	}
	return articleJson
}

type articleText struct {
	Cvid  int      `json:"cvid"`
	Up    string   `json:"up"`
	Title string   `json:"title"`
	Text  []string `json:"text"`
	seq   string   //不存本地
}

// 获取专栏作者、标题、正文
func getArticleText(cvid int) *articleText {
	if cacheAT, has := articleTextTable[cvid]; has && cacheAT != nil {
		log.Info("[bilibili] 调用缓存: cv", cvid)
		return cacheAT
	}
	body, err := ihttp.New().WithUrl(fmt.Sprint("https://www.bilibili.com/read/cv", cvid)).
		WithHeaders(iheaders).Get().ToString()
	if err != nil {
		log.Error("[bilibili] 专栏获取失败 ", err)
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		log.Error("[bilibili] 专栏解析失败 ", err)
		return nil
	}
	title := strings.TrimSpace(doc.Find("h1.title").First().Text())
	up := strings.TrimSpace(doc.Find("a.up-name").First().Text())
	main := doc.Find("#read-article-holder")
	text := []string{}
	main.Find("p, h1, h2, h3, h4, h5, h6").Each(func(_ int, el *goquery.Selection) {
		str := strings.TrimSpace(el.Text())
		if str != "" {
			text = append(text, str)
		}
	})
	at := &articleText{
		Cvid:  cvid,
		Up:    up,
		Title: title,
		Text:  text,
	}
	checkDir(tempDir)
	atByte, err := json.Marshal(at)
	if err != nil {
		log.Error("Cache Marshal err: ", err.Error())
	}
	localPath := fmt.Sprint(tempDir, "cv", cvid, ".json")
	os.WriteFile(localPath, atByte, 0644) //缓存
	at.marshal()
	return at
}

// 正文序列化
func (at *articleText) marshal() *articleText {
	at.seq = func() (seq string) {
		for _, str := range at.Text {
			if seq != "" {
				seq += "\n"
			}
			seq += str
		}
		return
	}()
	return at
}

// 格式化文章.Get("data")（文章信息拿不到自己的cv号）
func formatArticle(g gson.JSON, cvid int) string {
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
	reply := g.Get("stats.reply").Int()    //评论
	share := g.Get("stats.share").Int()    //分享
	like := g.Get("stats.like").Int()      //点赞
	coin := g.Get("stats.coin").Int()      //投币
	favor := g.Get("stats.favorite").Int() //收藏
	return fmt.Sprintf(
		`%s
cv%d
%s
作者：%s
%d阅读  %d评论  %d分享
%d点赞  %d投币  %d收藏
www.bilibili.com/read/cv%d`,
		images,
		cvid,
		title,
		author,
		view, reply, share,
		like, coin, favor,
		cvid)
}

// 获取音频数据.Get("data"), .Get("data"), .Get("data")
func getMusicJson(sid int) (gson.JSON, gson.JSON, gson.JSON) {
	musicJson, err := ihttp.New().WithUrl("https://www.bilibili.com/audio/music-service-c/web/song/info").
		WithAddQuerys(map[string]any{"sid": sid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getMusicJson().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawMusicJson: ", musicJson.JSON("", ""))
	if musicJson.Get("code").Int() != 0 {
		log.Error("[bilibili] 音频 ", sid, " 信息获取错误: ", musicJson.JSON("", ""))
	}
	tagJson, err := ihttp.New().WithUrl("https://www.bilibili.com/audio/music-service-c/web/tag/song").
		WithAddQuerys(map[string]any{"sid": sid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getMusicJson().tagJson.ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawMusicTagJson: ", tagJson.JSON("", ""))
	if tagJson.Get("code").Int() != 0 {
		log.Error("[bilibili] 音频tag ", sid, " 信息获取错误: ", tagJson.JSON("", ""))
	}
	stuffJson, err := ihttp.New().WithUrl("https://www.bilibili.com/audio/music-service-c/web/member/song").
		WithAddQuerys(map[string]any{"sid": sid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getMusicJson().stuffJson.ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawMusicstuffJson: ", stuffJson.JSON("", ""))
	if stuffJson.Get("code").Int() != 0 {
		log.Error("[bilibili] 音频tag ", sid, " 信息获取错误: ", stuffJson.JSON("", ""))
	}
	return musicJson, tagJson, stuffJson
}

var stuffMap = map[int]string{
	1:   "歌手",
	2:   "作词",
	3:   "作曲",
	4:   "编曲",
	5:   "后期/混音",
	7:   "封面制作",
	8:   "音源",
	9:   "调音",
	10:  "演奏",
	11:  "乐器",
	127: "UP主",
}

// 格式化音频.Get("data"), .Get("data"), .Get("data")
func formatMusic(g gson.JSON, h gson.JSON, i gson.JSON) string {
	cover := g.Get("cover").Str()       //封面
	sid := g.Get("id").Int()            //auid
	title := g.Get("title").Str()       //标题
	duration := g.Get("duration").Int() //时长(s)
	stuffs := func() (stuffs string) {  //成员列表, 职责：昵称
		for _, a := range i.Arr() {
			if stuffs != "" {
				stuffs += "\n"
			}
			stuffs += fmt.Sprintf("%s：%s",
				stuffMap[a.Get("type").Int()],
				a.Get("list.0.name").Str())
		}
		return
	}()
	tags := func() (tags string) { //标签列表
		for _, a := range h.Arr() {
			if tags != "" {
				tags += "、"
			}
			tags += a.Get("info").Str()
		}
		return
	}()
	intro := g.Get("intro").Str()             //简介
	play := g.Get("statistic.play").Int()     //播放
	coin := g.Get("coin_num").Int()           //投币
	reply := g.Get("statistic.comment").Int() //评论
	favor := g.Get("statistic.collect").Int() //收藏
	return fmt.Sprintf(
		`[CQ:image,file=%s]
au%d
%s
时长：%s
%s
标签：%s
简介：%s
%d播放  %d投币
%d评论  %d收藏`,
		cover,
		sid,
		title,
		formatTime(int64(duration)),
		stuffs,
		tags,
		intro,
		play, coin,
		reply, favor)
}

// 获取用户空间数据.Get("data.card")
func getSpaceJson(uid int) gson.JSON {
	spaceJson, err := ihttp.New().WithUrl("https://api.bilibili.com/x/web-interface/card").
		WithAddQuerys(map[string]any{"mid": uid}).WithHeaders(iheaders).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getSpaceJson().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawSpaceJson: ", spaceJson.JSON("", ""))
	if spaceJson.Get("code").Int() != 0 {
		log.Error("[parse] 空间 ", uid, " 信息获取错误: ", spaceJson.JSON("", ""))
	}
	return spaceJson
}

// 格式化空间.Get("data.card")
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

// uid获取直播间数据.Gets("data", strconv.Itoa(uid))
func getRoomJsonUID(uid int) (liveJson gson.JSON) {
	liveJson, err := ihttp.New().WithUrl("https://api.live.bilibili.com/room/v1/Room/get_status_info_by_uids").
		WithAddQuerys(map[string]any{"uids[]": uid}).WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getRoomJsonUID().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawRoomJson: ", liveJson.JSON("", ""))
	if liveJson.Get("code").Int() != 0 {
		log.Error("[parse] 直播间(UID) ", uid, " 信息获取错误: ", liveJson.JSON("", ""))
	}
	return
}

// 房间号获取直播间数据.Get("data")（拿不到UP用户名）
func getRoomJsonRoomid(roomid int) (liveJson gson.JSON) {
	liveJson, err := ihttp.New().WithUrl("https://api.live.bilibili.com/room/v1/Room/get_info").
		WithAddQuerys(map[string]any{"room_id": roomid}).WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getRoomJsonRoomID().ihttp请求错误: ", err)
	}
	log.Trace("[bilibili] rawRoomJson: ", liveJson.JSON("", ""))
	if liveJson.Get("code").Int() != 0 {
		log.Error("[parse] 直播间(RoomID) ", roomid, " 信息获取错误: ", liveJson.JSON("", ""))
	}
	return
}

// 格式化直播间
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
	history := func() (history string) {        //bot记录
		if liveList[g.Get("room_id").Int()].time != 0 {
			switch liveList[g.Get("room_id").Int()].state {
			case liveState.ONLINE:
				history = fmt.Sprintf("\n机器人缓存的上一次开播时间：\n%s",
					time.Unix(liveList[g.Get("room_id").Int()].time, 0).Format(timeLayout.M24C))
			case liveState.OFFLINE:
				history = fmt.Sprintf("\n机器人缓存的上一次下播时间：\n%s",
					time.Unix(liveList[g.Get("room_id").Int()].time, 0).Format(timeLayout.M24C))
			}
		}
		return
	}()
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

// 链接提取
func extractBiliLink(str string) (id string, kind string, summary bool, tts bool, upload bool) {
	short := regexp.MustCompile(biliLinkRegexp.SHORT).FindAllStringSubmatch(str, -1)
	dynamic := regexp.MustCompile(biliLinkRegexp.DYNAMIC).FindAllStringSubmatch(str, -1)
	av := regexp.MustCompile(biliLinkRegexp.ARCHIVEav).FindAllStringSubmatch(str, -1)
	bv := regexp.MustCompile(biliLinkRegexp.ARCHIVEbv).FindAllStringSubmatch(str, -1)
	cv := regexp.MustCompile(biliLinkRegexp.ARTICLE).FindAllStringSubmatch(str, -1)
	au := regexp.MustCompile(biliLinkRegexp.MUSIC).FindAllStringSubmatch(str, -1)
	space := regexp.MustCompile(biliLinkRegexp.SPACE).FindAllStringSubmatch(str, -1)
	live := regexp.MustCompile(biliLinkRegexp.LIVE).FindAllStringSubmatch(str, -1)
	log.Trace("[parse] short: ", short)
	log.Trace("[parse] dynamic: ", dynamic)
	log.Trace("[parse] av: ", av)
	log.Trace("[parse] bv: ", bv)
	log.Trace("[parse] cv: ", cv)
	log.Trace("[parse] au: ", au)
	log.Trace("[parse] space: ", space)
	log.Trace("[parse] live: ", live)
	sumTest := func(sumStr string) (summary bool) {
		if sumStr == "总结一下" {
			summary = true
		}
		return
	}
	ttsTest := func(ttsStr string) (tts bool) {
		if ttsStr == "让岁己" {
			tts = true
		}
		return
	}
	uploadTest := func(uploadStr string) (upload bool) {
		if uploadStr == "我要看" {
			upload = true
		}
		return
	}
	switch {
	case len(short) > 0:
		log.Debug("[parse] 识别到一个短链, short[0][4]: ", short[0][5])
		id = short[0][5]
		kind = "SHORT"
		tts = ttsTest(short[0][2])
		summary = sumTest(short[0][3])
		upload = uploadTest(short[0][1])
	case len(dynamic) > 0:
		log.Debug("[parse] 识别到一个动态, dynamic[0][4]: ", dynamic[0][4])
		id = dynamic[0][4]
		kind = "DYNAMIC"
		tts = ttsTest(dynamic[0][1])
		summary = sumTest(dynamic[0][2])
	case len(av) > 0:
		log.Debug("[parse] 识别到一个视频(av), av[0][4]: ", av[0][4])
		id = av[0][4]
		kind = "ARCHIVE"
		tts = ttsTest(av[0][2])
		summary = sumTest(av[0][3])
		upload = uploadTest(av[0][1])
	case len(bv) > 0:
		log.Debug("[parse] 识别到一个视频(bv), bv[0][4]: ", bv[0][4])
		id = strconv.Itoa(bv2av(bv[0][4]))
		kind = "ARCHIVE"
		tts = ttsTest(bv[0][2])
		summary = sumTest(bv[0][3])
		upload = uploadTest(bv[0][1])
	case len(cv) > 0:
		log.Debug("[parse] 识别到一个专栏, cv[0][4]: ", cv[0][4])
		id = cv[0][4]
		kind = "ARTICLE"
		tts = ttsTest(cv[0][1])
		summary = sumTest(cv[0][2])
	case len(au) > 0:
		log.Debug("[parse] 识别到一个音频, au[0][3]: ", au[0][3])
		id = au[0][3]
		kind = "MUSIC"
		tts = ttsTest(au[0][1])
		summary = sumTest(au[0][2])
	case len(space) > 0:
		log.Debug("[parse] 识别到一个用户空间, space[0][3]: ", space[0][3])
		id = space[0][3]
		kind = "SPACE"
		tts = ttsTest(space[0][1])
		summary = sumTest(space[0][2])
	case len(live) > 0:
		log.Debug("[parse] 识别到一个直播, live[0][3]: ", live[0][3])
		id = live[0][3]
		kind = "LIVE"
		tts = ttsTest(live[0][1])
		summary = sumTest(live[0][2])
	}
	return
}

// 短时间重复解析屏蔽, op:=true
func isBiliLinkOverParse(ctx *EasyBot.CQMessage, id string, kind string) bool {
	if ctx.MessageType == "group" { //只有群聊有限制
		duration := int64(v.GetFloat64("parse.settings.sameParseInterval"))
		during := time.Now().Unix()-groupParseHistory[ctx.GroupID].time < duration
		same := id == groupParseHistory[ctx.GroupID].parse
		block := during && same
		if ctx.IsSU() {
			block = false
		}
		if block {
			log.Info("[parse] 在群 ", ctx.GroupID, " 屏蔽了一次小于 ", duration, " 秒的相同解析 ", kind, " ", id)
			return true
		} else {
			log.Debug("[parse] 记录了一次在 ", ctx.GroupID, " 的解析 ", id)
			groupParseHistory[ctx.GroupID] = parseHistory{ //记录解析历史
				parse: id,
				time:  time.Now().Unix(),
			}
		}
	}
	return false
}

type dynamicContent struct {
	up   string
	text string
}

// 内容解析并格式化
func parseAndFormatBiliLink(ctx *EasyBot.CQMessage, id, kind string, summary, tts, upload bool) (content string) {
	var op bool
	if ctx != nil {
		op = isBiliLinkOverParse(ctx, id, kind)
	} else {
		op, summary = false, false
	}
	if !summary { //需要总结时不检测屏蔽，到最后再清空content
		if op {
			return
		}
	}
	switch kind {
	case "":
	case "SHORT":
		id, kind, _, _, _ := extractBiliLink(deShortLink(id))
		content = parseAndFormatBiliLink(ctx, id, kind, summary, tts, upload)
	case "DYNAMIC":
		g := getDynamicJson(id)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 动态%s信息获取错误: code%d", id, g.Get("code").Int())
		} else {
			content = formatDynamic(g.Get("data.item"))
			if summary {
				go func() {
					dc := &dynamicContent{
						up:   g.Get("data.item.modules.module_author.name").Str(),
						text: g.Get("data.item.modules.module_dynamic.desc.text").Str(),
					}
					s := dc.summary()
					ctx.SendMsg(s)
					if tts {
						sendVitsMsg(ctx, rmTitle.Replace(s), "ZH") //不念“概述”、“要点”
					}
				}()
			}
		}
	case "ARCHIVE":
		aid, _ := strconv.Atoi(id)
		g, h := getArchiveJson(aid)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 视频av%s信息获取错误: code%d", id, g.Get("code").Int())
		} else {
			content = formatArchive(g.Get("data"), h.Get("data"))
			sum, err := getArchiveSummary(aid)
			if err != nil {
				log.Error("[NothingBot] 总结获取错误 err: ", err)
			} else if sum != "" {
				content += "\n\n哔哩哔哩AI总结：\n" + sum
			}
			if summary {
				go func() {
					var as *archiveSubtitle
					if cache, hasCache := archiveSubtitleTable[aid]; hasCache {
						as = cache
					} else {
						as = getSubtitle(aid, g.Get("data.owner.name").Str(), g.Get("data.title").Str())
						if as == nil {
							ctx.SendMsgReply("[NothingBot] [Info] 无法获取视频字幕，尝试调用BcutASR")
							as = bcutSubtitle(aid, g.Get("data.owner.name").Str(), g.Get("data.title").Str())
						}
					}
					if as != nil {
						archiveSubtitleTable[aid] = as //缓存字幕
						s := as.summary()
						ctx.SendMsg(s)
						if tts {
							sendVitsMsg(ctx, rmTitle.Replace(s), "ZH") //不念“概述”、“要点”
						}
					} else {
						ctx.SendMsgReply("[NothingBot] [Error] 字幕转录失败力")
					}
				}()
			}
			if upload {
				go func() {
					var av *archiveVideo
					if cache, hasCache := archiveVideoTable[aid]; hasCache {
						av = cache
					} else {
						av = getVideoMp4(aid, videoQual.gq720)
					}
					if av != nil {
						archiveVideoTable[aid] = av //缓存视频
						ctx.SendMsg(bot.Utils.Format.Video(av.path))
					} else {
						ctx.SendMsgReply("[NothingBot] [Error] 视频获取失败力")
					}
				}()
			}
		}
	case "ARTICLE":
		cvid, _ := strconv.Atoi(id)
		g := getArticleJson(cvid)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [Error] [parse] 专栏cv%d信息获取错误: code%d", cvid, g.Get("code").Int())
		} else {
			content = formatArticle(g.Get("data"), cvid) //专栏信息拿不到自身cv号
			if summary {
				go func() {
					var at *articleText
					if cache, hasCache := articleTextTable[cvid]; hasCache {
						at = cache
					} else {
						at = getArticleText(cvid)
					}
					if at != nil {
						articleTextTable[cvid] = at //缓存专栏
						s := at.summary()
						ctx.SendMsg(s)
						if tts {
							sendVitsMsg(ctx, rmTitle.Replace(s), "ZH") //不念“概述”、“要点”
						}
					} else {
						ctx.SendMsgReply("[NothingBot] 文章正文获取失败力")
					}
				}()
			}
		}
	case "MUSIC":
		sid, _ := strconv.Atoi(id)
		g, h, i := getMusicJson(sid)
		if g.Get("code").Int() != 0 || h.Get("code").Int() != 0 || i.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [Error] [parse] 音频au%d信息获取错误: codes: %d, %d, %d", sid, g.Get("code").Int(), h.Get("code").Int(), i.Get("code").Int())
		} else {
			content = formatMusic(g.Get("data"), h.Get("data"), i.Get("data"))
			if summary {
				go func() {
					time.Sleep(time.Second * 2)
					ctx.SendMsgReply("没做")
				}()
			}
		}
	case "SPACE":
		uid, _ := strconv.Atoi(id)
		g := getSpaceJson(uid)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [Error] [parse] 用户%s信息获取错误: code%d", id, g.Get("code").Int())
		} else {
			content = formatSpace(g.Get("data.card"))
			if summary {
				go func() {
					time.Sleep(time.Second * 2)
					ctx.SendMsgReply("？")
				}()
			}
		}
	case "LIVE":
		id, _ := strconv.Atoi(id)
		uid := getRoomJsonRoomid(id).Get("data.uid").Int()
		if uid != 0 {
			roomJson, ok := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
			if ok {
				content = formatLive(roomJson)
				if summary {
					go func() {
						time.Sleep(time.Second * 2)
						ctx.SendMsgReply("？？？")
					}()
				}
			} else {
				content = fmt.Sprintf("[NothingBot] [Error] [parse] 直播间%d信息获取错误, !ok", id)
			}
		} else {
			content = fmt.Sprintf("[NothingBot] [Error] [parse] 直播间%d信息获取错误, uid == \"0\"", id)
		}
	}
	if op {
		return ""
	}
	return
}

// 短链解析
func deShortLink(slug string) (location string) {
	header, err := ihttp.New().WithUrl("https://b23.tv/" + slug).
		WithHijackRedirect().Head().ToHeader()
	if err != nil {
		log.Error("[parse] deShortLink().ihttp请求错误: ", err)
	}
	if len(header["Location"]) > 0 {
		location = header["Location"][0]
	}
	var statusCode string
	if len(header["Bili-Status-Code"]) > 0 {
		statusCode = header["Bili-Status-Code"][0]
	}
	switch statusCode {
	case "-404":
		log.Warn("[parse] 短链解析失败: ", statusCode, "  location: ", location)
	}
	return
}

// 根据config选择后端
func chatModelSummary(input string) (output string, err error) {
	log.Debug("[summary] backend: ", summaryBackend)
	switch summaryBackend {
	case "glm":
		output, err = sendToChatGLMSingle(input)
		output = "由" + selectedModelStr + "总结：\n" + output
		if err != nil {
			log.Error("[summary] ChatGLM2 err: ", err)
		}
	case "qianfan":
		var overLen bool
		if len([]rune(input)) > 1500 {
			input = string([]rune(input)[:1499])
			overLen = true
		}
		output, err = sendToWenxinSingle(input)
		if !overLen {
			output = "由" + selectedModelStr + "总结：\n" + output
		} else {
			output = "由" + selectedModelStr + "总结（原文长度超过1500字符，输入经过去尾）：\n" + output
		}
		if err != nil {
			log.Error("[summary] qianfan err: ", err)
		}
	}
	return
}

// 总结模板
func getPrompt(kind string, title string, up string, seq string) string {
	kindList := map[string]string{
		"archive": "视频字幕",
		"article": "专栏文章",
		"dynamic": "空间动态",
	}
	//return "使用以下Markdown模板为我总结" + kindList[kind] + "，除非" + kindList[kind][6:] + "中的内容无意义，或者未提供" + kindList[kind][6:] + "内容，或者内容较少无法总结，或者无有效内容，你就不使用模板回复，只回复“无意义”。" +
	return "使用以下Markdown模板为我总结" + kindList[kind] + "，除非内容较少无法总结，你就不使用模板回复，只回复“内容过少，无法总结”。" +
		"\n## 概述" +
		"\n{尽可能精简总结内容不要太详细}" +
		"\n## 要点" +
		"\n- {不换行、大于15字、可多项、条数与有效内容数量呈正比}" +
		"\n不要随意翻译任何内容。仅使用中文总结。" +
		"\n不说与总结无关的其他内容，你的回复仅限固定格式提供的“概述”和“要点”两项。" +
		func() (s string) {
			s += "\n" + kindList[kind][:6]
			switch kind {
			case "archive":
				s += "标题为《" + title + "》，"
			case "article":
				s += "标题为《" + title + "》，发布者为" + up + "，"
			case "dynamic":
				s += "发布者为" + up + "，"
			}
			s += kindList[kind][6:] + "数据如下，立刻开始总结："
			return
		}() +
		"\n" + seq
}

// 总结视频
func (as *archiveSubtitle) summary() (output string) {
	input := getPrompt("archive", as.Title, as.Up, as.seq)
	output, err := chatModelSummary(input)
	if err != nil {
		output = "[NothingBot] [Error] [summary] " + err.Error()
	}
	return
}

// 总结文章
func (at *articleText) summary() (output string) {
	input := getPrompt("article", at.Title, at.Up, at.seq)
	output, err := chatModelSummary(input)
	if err != nil {
		output = "[NothingBot] [Error] [summary] " + err.Error()
	}
	return
}

// 总结动态
func (dc *dynamicContent) summary() (output string) {
	input := getPrompt("dynamic", "", dc.up, dc.text)
	output, err := chatModelSummary(input)
	if err != nil {
		output = "[NothingBot] [Error] [summary] " + err.Error()
	}
	return
}
