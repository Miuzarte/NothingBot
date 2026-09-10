package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/robfig/cron/v3"
)

const APEX_MAP_PUSH_REGEXP = `(?i)(当前)?apex(当前)?地图`

var apexMapPushReg = regexp.MustCompile(APEX_MAP_PUSH_REGEXP)

type ApexMapResp struct {
	Success    bool   `json:"success"`
	GoodMap    bool   `json:"goodMap"`
	CurrentMap string `json:"currentMap"`
	NextMap    string `json:"nextMap"`
	PlainText  string `json:"plainText"`
	AlertMsg   string `json:"alertMsg"`
}

type ApexMapPushConfig struct {
	ApiUrl        string
	Authorization string
	Crontab       string

	Groups []int
	Users  []int

	Specials map[string][]int
	specials map[int][]int // group -> users to at

	cronId   cron.EntryID
	spGroups []int
	trigger  chan *EasyOnebot.Ctx // 手动触发
	closed   uintptr
}

const apexMapPushMId ModuleId = "ApexMapPush"

var (
	apexMapPushConfig *ApexMapPushConfig
	apexMapPushCron   = cron.New()
)

var moduleApexMapPush = Module{
	ModuleMeta: ModuleMeta{
		Name:       apexMapPushMId,
		Desc:       "Apex排位地图推送",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
		HelpMsg:    APEX_MAP_PUSH_REGEXP,
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_ApexMapPush.go")

	moduleApexMapPush.AfterInit = initApexMapPush
	moduleApexMapPush.ReInit = initApexMapPush
	moduleApexMapPush.AtExit = func() {
		if apexMapPushConfig != nil {
			apexMapPushConfig.Stop()
		}
	}
	modules.Add(&moduleApexMapPush)
}

func initApexMapPush() {
	newConfig := &ApexMapPushConfig{}
	err := config.DecodeModule(apexMapPushMId, newConfig)
	if err != nil {
		log.Error(err)
		return
	}

	<-apexMapPushCron.Stop().Done() // 停止 cron
	if apexMapPushConfig != nil {
		apexMapPushConfig.Stop() // 停止旧的配置
		if apexMapPushConfig.cronId != 0 {
			apexMapPushCron.Remove(apexMapPushConfig.cronId)
		}
	}
	apexMapPushConfig = newConfig

	if apexMapPushConfig.Authorization == "" {
		log.Error("[ApexMapPush] Authorization is empty")
		return
	}

	if apexMapPushConfig.Crontab != "" {
		apexMapPushConfig.cronId, err = apexMapPushCron.AddFunc(
			apexMapPushConfig.Crontab,
			// nil for push all
			func() { ctxApexMapPush(nil) },
		)
		if err != nil {
			log.Error("[ApexMapPush] failed to add cron: ", err)
			return
		}
	}

	apexMapPushConfig.specials = make(map[int][]int, len(apexMapPushConfig.Specials))
	for k, v := range apexMapPushConfig.Specials {
		gId, err := strconv.Atoi(k)
		if err != nil {
			log.Error("[ApexMapPush] failed to parse group id: ", err)
			continue
		}
		apexMapPushConfig.specials[gId] = v
	}
	apexMapPushConfig.spGroups = make([]int, 0, len(apexMapPushConfig.specials))
	for k := range apexMapPushConfig.specials {
		apexMapPushConfig.spGroups = append(apexMapPushConfig.spGroups, k)
	}

	apexMapPushConfig.trigger = make(chan *EasyOnebot.Ctx)
	apexMapPushConfig.closed = 0

	apexMapPushConfig.Start() // 开始监听信号
	apexMapPushCron.Start()   // 启动 cron
	for i, ent := range apexMapPushCron.Entries() {
		log.Debugf("[ApexMapPush] crontab[%d](%d) next: %s", i, ent.ID, ent.Next.Format(time.DateTime))
	}

	if len(apexMapPushConfig.spGroups) == 0 {
		onebot.AddMatcher(moduleApexMapPush.Name.String(), EasyOnebot.NewMatcher().
			OnTypeL1(event.TYPE_L1_MESSAGE).
			OnRegexpMatchString(apexMapPushReg).
			Do(moduleApexMapPush.RWMuWrap(ctxApexMapPush)),
		)
	} else {
		onebot.AddMatcher(moduleApexMapPush.Name.String(), EasyOnebot.NewMatcher().
			OnTypeL1(event.TYPE_L1_MESSAGE).
			IsNotGroup(apexMapPushConfig.spGroups...).
			OnRegexpMatchString(apexMapPushReg).
			Do(moduleApexMapPush.RWMuWrap(ctxApexMapPush)),
		)
		onebot.AddMatcher(moduleApexMapPush.Name.String()+"_SP", EasyOnebot.NewMatcher().
			OnTypeL1(event.TYPE_L1_MESSAGE).
			IsGroup(apexMapPushConfig.spGroups...).
			OnStringsContains("傻狗要看apex地图").
			Do(moduleApexMapPush.RWMuWrap(ctxApexMapPush)),
		)
	}
}

func ctxApexMapPush(ctx *EasyOnebot.Ctx) {
	apexMapPushConfig.trigger <- ctx
}

func (ampc *ApexMapPushConfig) Start() {
	go func() {
		for ctx := range ampc.trigger {
			go ampc.tryPushApexMapUpdate(ctx)
			for i, ent := range apexMapPushCron.Entries() {
				log.Debugf("[ApexMapPush] crontab[%d](%d) next: %s", i, ent.ID, ent.Next.Format(time.DateTime))
			}
		}
	}()
}

func (ampc *ApexMapPushConfig) Stop() {
	if atomic.CompareAndSwapUintptr(&ampc.closed, 0, 1) {
		close(ampc.trigger)
	}
}

func (ampc *ApexMapPushConfig) tryPushApexMapUpdate(ctx *EasyOnebot.Ctx) {
	var resp ApexMapResp
	var err error
	for range 5 { // retry for 5 times
		resp, err = getApexMapUpdate(ampc.ApiUrl, ampc.Authorization)
		if err != nil {
			log.Warn("[ApexMapPush] failed to get map update: ", err)
			goto FAILED
		}
		if !resp.Success {
			log.Warn("[ApexMapPush] failed to get map update: ", resp)
			goto FAILED
		}

		ampc.pushApexMapUpdate(resp, ctx)
		break

	FAILED:
		<-time.After(20 * time.Second)
	}
}

func (ampc *ApexMapPushConfig) pushApexMapUpdate(resp ApexMapResp, ctx *EasyOnebot.Ctx) {
	spGroup := ctx == nil || slices.Contains(ampc.spGroups, ctx.Event.GroupId)

	msg := fmt.Sprintf("Apex Legends\n当前地图：%s\n下个地图：%s\n\n%s", resp.CurrentMap, resp.NextMap, resp.PlainText)

	if ctx != nil { // 手动触发
		if !spGroup { // 跳过 sp
			_, err := ctx.SendMsgReply(msg)
			if err != nil {
				log.Warn("[ApexMapPush] failed to send msg reply: ", err)
			}
		}
	} else { // 定时推送
		PushMsg(msg, apexMapPushConfig.Users, apexMapPushConfig.Groups)
	}

	if !spGroup {
		return
	}
	for gId, uIds := range apexMapPushConfig.specials {
		if len(uIds) == 0 {
			continue
		}
		segChain := make(message.SegmentArray, 0, len(uIds)+1)
		if resp.GoodMap {
			for _, uId := range uIds {
				segChain.Append(message.At(uId))
			}
		}
		segChain.Append(message.Text(resp.AlertMsg))
		_, err := onebot.Call().Std.SendGroupMsg(gId, segChain)
		if err != nil {
			log.Warn("[ApexMapPush] failed to send group msg: ", err)
		}
	}
}

func getApexMapUpdate(apiUrl, auth string) (amr ApexMapResp, err error) {
	req, err := http.NewRequest(http.MethodGet, apiUrl, nil)
	if err != nil {
		log.Panic(err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	log.Debug("[ApexMapPush] resp: ", string(body))

	err = json.Unmarshal(body, &amr)
	if err != nil {
		return
	}

	return amr, nil
}
