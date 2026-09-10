package main

import (
	"fmt"
	"regexp"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/Miuzarte/biligo"
)

// 本文件的日志 scope
var logBiliSearch = logger.New("BiliSearch")

const (
	BILI_SEARCH_TYPE                    = `(视频|番剧|电影|直播间|直播|主播|专栏|用户)`
	BILI_SEARCH_REGEXP                  = `(?si)^B站?搜索?\s*` + BILI_SEARCH_TYPE + `?` + `[\s:：]*(.+)`
	BILI_SEARCH_REGEXP_INDEX_SEARCHTYPE = 1
	BILI_SEARCH_REGEXP_INDEX_KEYWORD    = 2
)

var biliSearchReg = regexp.MustCompile(BILI_SEARCH_REGEXP)

const biliSearchMId ModuleId = "BiliSearch"

var moduleBiliSearch = Module{
	ModuleMeta: ModuleMeta{
		Name:       biliSearchMId,
		Desc:       "B站搜索\n边水群边刷B站，这辈子就在这了",
		Conditions: Conditions{},
		HelpMsg:    "综合搜索：B(站)搜 <关键字>\n分类搜索：B(站)搜 <视频|番剧|电影|直播间|直播|主播|专栏|用户> <关键字>",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_BiliSearch.go")

	moduleBiliSearch.Init = initBiliSearch
	modules.Add(&moduleBiliSearch)
}

func initBiliSearch() {
	onebot.AddMatcher(moduleBiliSearch.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotForwardMsg().
		OnRegexpFindAllStringSubmatch(biliSearchReg).
		Do(moduleBiliSearch.RWMuWrap(ctxBiliSearch)),
	)
}

func ctxBiliSearch(ctx *EasyOnebot.Ctx) {
	submatch := ctx.Submatches.Get(moduleBiliSearch.Name.String())[0]
	searchTypeRaw, keyword := submatch[BILI_SEARCH_REGEXP_INDEX_SEARCHTYPE], submatch[BILI_SEARCH_REGEXP_INDEX_KEYWORD]
	searchType, ok := biligo.SearchTypePam[searchTypeRaw]
	if searchTypeRaw != "" && !ok {
		ctx.SendMsgf("未知类型：%s，请使用 %s", searchTypeRaw, BILI_SEARCH_TYPE)
		return
	}

	if searchTypeRaw == "" {
		searchTypeRaw = "综合"
	} // 方便后续输出

	resp, err := ctx.SendMsgf("正在执行%s搜索...", searchTypeRaw)
	if err != nil {
		logBiliSearch.Error().
			Err(err).
			Msg("failed to send message")
		return
	}
	defer ctx.DeleteMsg(resp.MessageID)

	results, err := biligo.SearchFormatAuto(searchType, keyword)
	if err != nil {
		ctx.SendMsgf("搜索失败：%v", err)
		return
	}
	if len(results) == 0 {
		ctx.SendMsgReply("未搜到相关内容")
		return
	}

	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()
	reply := message.SegmentArray{
		message.Node3(uid, name, fmt.Sprintf("%s搜索：%s\n共%d个结果", searchTypeRaw, keyword, len(results))),
	}
	for _, result := range results {
		reply.Append(
			message.Node3(uid, name, result.DoTemplate()),
		)
	}
	ctx.SendForwardMsgAuto(reply)
}
