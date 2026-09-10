package main

import (
	"io"
	"net/http"

	"NothingBot_v4/RSSHub/EpicFree"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/robfig/cron/v3"
)

type RssHubResource interface {
	GetRouter() string
	GetCrontab() string
}

const (
	// RSSHUB_URL = `https://rsshub.miuzarte.top/`
	RSSHUB_URL = `http://127.0.0.1:7200`
	RSSHUB_KEY = `114514`
)

var (
	rssCron = cron.New()
	// TODO: 统一的定时 or 单独的定时
	cronIds         = make(map[RssHubResource]cron.EntryID) // resource -> cronId
	rssHubResources = []RssHubResource{                     // TODO: 自动化注册
		EpicFree.Resource,
	}
)

const rssHubMId ModuleId = "RssHub"

// rssHubConfig = RssHubConfig{}

var moduleRssHub = Module{
	ModuleMeta: ModuleMeta{
		Name:       rssHubMId,
		Desc:       "RssHub",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
	},
	Disable: env.Testing,
}

func initRH() {
	NoBuildPrintFile("M_RssHub.go")

	moduleRssHub.AfterInit = initRssHub
	// moduleRssHub.ReInit = initRssHub
	modules.Add(&moduleRssHub)
}

func initRssHub() {
	cronId, err := rssCron.AddFunc(EpicFree.Resource.Crontab, pushRssEpicFree)
	if err != nil {
		log.Fatal(err)
	}
	cronIds[EpicFree.Resource] = cronId
	// rssCron.Start() // TODO;

	// go func() {
	// }()
	pushRssEpicFree()
}

func pushRssEpicFree() {
	resp, err := http.Get(RSSHUB_URL + EpicFree.Resource.GetRouter() + "?key=" + RSSHUB_KEY)
	if err != nil {
		onebot.Log2Sus.Errorf("RssHub: EpicFree http.Get error: %v", err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		onebot.Log2Sus.Errorf("RssHub: EpicFree io.ReadAll error: %v", err)
	}
	feeds, err := EpicFree.ParseFeed(data)
	if err != nil {
		onebot.Log2Sus.Errorf("RssHub: EpicFree ParseFeed error: %v", err)
	}

	li, err := onebot.Call().Std.GetLoginInfo()
	if err != nil {
		log.Fatal(err)
	}
	uid := li.UserId

	forward := message.SegmentArray{}
	forward.Append(message.Node3(uid, "", "Epic 每周免费游戏"))
	for _, feed := range feeds {
		forward.Append(message.Node3(uid, "", feed.String()))
	}

	// PushMsg(forward, nil, []int{658359592})
	err = PushMsg(forward, nil, []int{612645549})
	if err == nil {
		// push success
		// 写入到 redis
		// key: hash(游戏名+游戏名)
		// value: 发送时间
	}
}
