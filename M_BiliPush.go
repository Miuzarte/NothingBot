package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/biligo"

	// "github.com/gorilla/websocket"
	"github.com/coder/websocket"
	"github.com/tidwall/gjson"
)

const (
	BILIPUSH_DYNAMIC_UPDATE_INTERVAL_MIN = time.Second
)

type BiliPushConfig struct {
	DynamicUpdateInterval string // 动态更新拉取间隔
	dynamicUpdateInterval time.Duration
	LiveMinimumInterval   string // 同一直播间多次开播推送的最小间隔, 用于解决某些主播因网络问题频繁重新推流导致多次推送
	liveMinimumInterval   time.Duration

	List    []*BiliPushConfigList
	listMap map[int]*BiliPushConfigList // uid: config

	dynamicLoopCtx    context.Context
	dynamicLoopCancel context.CancelFunc
}

const biliPushMId ModuleId = "BiliPush"

var biliPushConfig = BiliPushConfig{}

var moduleBiliPush = Module{
	ModuleMeta: ModuleMeta{
		Name:       biliPushMId,
		Desc:       "Bilibili动态、直播推送",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_BiliPush.go")

	moduleBiliPush.AfterInit = initBiliPush // 防止没初始化完就收到推送
	moduleBiliPush.ReInit = initBiliPush
	moduleBiliPush.AtExit = func() { biliPushConfig.StopAll() }
	modules.Add(&moduleBiliPush)
}

func initBiliPush() {
	newConfig := BiliPushConfig{}
	err := config.DecodeModule(biliPushMId, &newConfig)
	if err != nil {
		log.Error(err)
		return
	}

	biliPushConfig.StopAll()
	biliPushConfig = newConfig

	biliPushConfig.dynamicUpdateInterval, err = time.ParseDuration(biliPushConfig.DynamicUpdateInterval)
	if err != nil {
		log.Error("[BiliPush] failed to parse dynamic update interval: ", err)
		biliPushConfig.dynamicUpdateInterval = time.Second * 3
	}
	if biliPushConfig.dynamicUpdateInterval < BILIPUSH_DYNAMIC_UPDATE_INTERVAL_MIN {
		log.Warnf("[BiliPush] dynamic update interval too short, set to %s", BILIPUSH_DYNAMIC_UPDATE_INTERVAL_MIN)
		biliPushConfig.dynamicUpdateInterval = BILIPUSH_DYNAMIC_UPDATE_INTERVAL_MIN
	}

	biliPushConfig.liveMinimumInterval, err = time.ParseDuration(biliPushConfig.LiveMinimumInterval)
	if err != nil {
		log.Error("[BiliPush] failed to parse live minimum interval: ", err)
		biliPushConfig.liveMinimumInterval = time.Second * 300
	}

	if biliPushConfig.listMap == nil {
		biliPushConfig.listMap = make(map[int]*BiliPushConfigList)
	}
	for _, list := range biliPushConfig.List {
		biliPushConfig.listMap[list.Uid] = list
	}

	err = biliPushConfig.GetInfo()
	if err != nil {
		log.Error("[BiliPush] failed to get info: ", err)
	}

	// go biliPushConfig.RunLive()
	// go biliPushConfig.RunDynamic()
}

func (bpc *BiliPushConfig) GetInfo() (err error) {
	for i, list := range bpc.List {
		err = list.GetInfo()
		if err != nil {
			log.Errorf("[BiliPush] failed to get info for [%d]%d: %s", i, list.Uid, err)
			log.Debugf("%+v", list)
			return err
		}
	}
	return nil
}

func (bpc *BiliPushConfig) RunLive() {
	l := 0
	for _, list := range bpc.List {
		if list.pushLive {
			l++
			go list.ListenLive()
		}
	}
	log.Info("[BiliPush] live listening: ", l)
}

func (bpc *BiliPushConfig) RunDynamic() {
	log.Info("[BiliPush] dynamic listening: ", len(bpc.List))
	if len(bpc.List) == 0 {
		return
	}

	if bpc.dynamicLoopCancel != nil {
		bpc.dynamicLoopCancel()
	}
	bpc.dynamicLoopCtx, bpc.dynamicLoopCancel = context.WithCancel(context.Background())

	biliPushHistoryDynamic.Init(2 * len(bpc.List))

	var err error
	var da biligo.DynamicAll
	var dau biligo.DynamicAllUpdate

	for { // 无限尝试获取 baseline
		da, err = biligo.FetchDynamicAll()
		if err != nil {
			log.Error("[BiliPush] failed to fetch dynamic all: ", err)
			select {
			case <-time.After(time.Second * 10):
				continue
			case <-bpc.dynamicLoopCtx.Done():
				return
			}
		}
		if da.UpdateBaseline == "" {
			log.Error("[BiliPush] update baseline is empty")
			select {
			case <-time.After(time.Second * 10):
				continue
			case <-bpc.dynamicLoopCtx.Done():
				return
			}
		}
		break
	}

	for { // 拉取更新
		if da.UpdateBaseline == "" {
			go biliPushConfig.RunDynamic() // 重新拉取 baseline
			log.Warn("[BiliPush] update baseline is empty, restarting dynamic loop")
			return
		}

		dau, err = biligo.FetchDynamicAllUpdate(da.UpdateBaseline)
		if err != nil {
			log.Error("[BiliPush] failed to fetch dynamic all update: ", err)
			goto FAILED
		}
		if dau.UpdateNum == 0 {
			goto WAIT
		}

		log.Debug("[BiliPush] new dynamic: ", dau.UpdateNum)
		for range 3 { // 失败 3 次放弃推送
			da, err = biligo.FetchDynamicAll()
			if err == nil {
				break
			}
			<-time.After(time.Second * 10)
		}
		if err != nil {
			log.Error("[BiliPush] failed to fetch dynamic all: ", err)
			goto FAILED
		}
		if len(da.Items) == 0 {
			log.Error("[BiliPush] dynamic items is empty")
			goto FAILED
		}

		for i := range da.Items {
			if i >= dau.UpdateNum {
				break // 更新了几条就拉几条
			}

			mid := da.Items[i].Modules.Author.Mid
			list, ok := bpc.listMap[mid]
			if !ok || !list.pushDynamic {
				log.Debug("[BiliPush] skipping dynamic: ", mid, ok, list)
				continue
			}
			if len(list.Filter) > 0 &&
				!slices.Contains(list.Filter, da.Items[i].Type) {
				log.Debug("[BiliPush] skipping dynamic: ", da.Items[i].Type, list.Filter)
				continue
			}
			if biliPushHistoryDynamic.Query(da.Items[i].IdStr) {
				log.Debug("[BiliPush] skipping dynamic: ", da.Items[i].IdStr)
				continue
			}
			biliPushHistoryDynamic.Add(da.Items[i].IdStr)

			log.Debugf("[BiliPush] pushing dynamic %s to %v %v", da.Items[i].IdStr, list.Groups, list.Users)
			PushMsg(da.Items[i].DoTemplate(), list.Users, list.Groups)
		}

	WAIT:
		select {
		case <-time.After(bpc.dynamicUpdateInterval):
			continue
		case <-bpc.dynamicLoopCtx.Done():
			return
		}

	FAILED:
		<-time.After(time.Second * 10)
		continue
	}
}

func (bpc *BiliPushConfig) StopAll() {
	if bpc.dynamicLoopCancel != nil {
		bpc.dynamicLoopCancel()
	}
	for _, list := range bpc.List {
		if list.lms != nil {
			list.lms.Stop()
		}
	}
}

type BiliPushConfigList struct {
	Uid    int   // uid
	Live   int   // 直播间号
	Groups []int // 推送的用户
	Users  []int // 推送的群组
	Filter []string

	name        string // up 名
	title       string // 直播间标题
	pushDynamic bool   // .Uid != 0
	pushLive    bool   // .Live != 0
	lms         *biligo.LiveMsgStream
}

// GetInfo 根据 uid 或 roomId 拿信息
func (bpl *BiliPushConfigList) GetInfo() (err error) {
	bpl.pushDynamic = bpl.Uid != 0
	bpl.pushLive = bpl.Live != 0
	if !bpl.pushDynamic && !bpl.pushLive {
		return errors.New("both uid and live are empty")
	}
	if bpl.pushDynamic {
		lsu, err := biligo.FetchLiveStatus(Itoa(bpl.Uid))
		// 没开通直播间时 json 解析也会返回错误
		if err == nil {
			ls := lsu[Itoa(bpl.Uid)]
			if ls == nil || ls.Title == "" {
				return fmt.Errorf("failed to get title: %v", lsu)
			}
			bpl.title = ls.Title
		}
	}
	if bpl.pushLive {
		lri, err := biligo.FetchLiveRoomInfo(Itoa(bpl.Live))
		if err != nil {
			return err
		}
		bpl.title = lri.Title
		if bpl.title == "" {
			return fmt.Errorf("failed to get title: %v", lri)
		}
		if lri.Uid == 0 {
			return fmt.Errorf("failed to get uid: %v", lri)
		}
		if bpl.Uid == 0 { // 仍需要 uid 以获取用户名
			bpl.Uid = lri.Uid
		}
	}

	// 拿用户名
	sc, err := biligo.FetchSpaceCard(Itoa(bpl.Uid))
	if err != nil {
		return err
	}
	bpl.name = sc.Card.Name
	if bpl.name == "" {
		return fmt.Errorf("failed to get name: %v", sc)
	}
	return nil
}

func (bpl *BiliPushConfigList) ListenLive() {
	log.Debugf("[BiliPush] listening live msg: %s %d %d", bpl.name, bpl.Uid, bpl.Live)
	defer log.Debugf("[BiliPush] live msg stream closed: %s %d %d", bpl.name, bpl.Uid, bpl.Live)
	bpl.lms = biligo.NewLiveMsgStream(bpl.Live)
	for body, err := range bpl.lms.RunIter() {
		if err != nil {
			switch websocket.CloseStatus(err) {
			case websocket.StatusNormalClosure,
				websocket.StatusAbnormalClosure:
				return
			}
			log.Error("[BiliPush] failed to listen live msg: ", err)
			<-time.After(time.Second * 10)
			go bpl.ListenLive() // 重连
			return
		}
		bpl.LiveMsgHandler(body)
	}
}

func (bpl *BiliPushConfigList) LiveMsgHandler(body string) {
	pkt := gjson.Parse(body)

	miss := false
	cmd := pkt.Get("cmd").String()
	switch cmd {
	case biligo.LIVE_MSG_STREAM_LIVE: // 直播开始
		bpl.PushOnline(&pkt)

	case biligo.LIVE_MSG_STREAM_PREPARING: // 直播准备中 (结束)
		bpl.PushOffline(&pkt)

	case biligo.LIVE_MSG_STREAM_CHANGE: // 房间信息变更
		bpl.PushChange(&pkt)

	case biligo.LIVE_MSG_STREAM_WARNING: // 警告
		bpl.PushWarning(&pkt)

	case biligo.LIVE_MSG_STREAM_CUT_OFF: // 切断
		bpl.PushCutOff(&pkt)

	default:
		// log.Debugf("[BiliPush] unknown live msg cmd: %s\n%s", cmd, body)
		miss = true
	}

	if !miss {
		log.Debugf("[BiliPush] %s(%d %d) live %s", bpl.name, bpl.Uid, bpl.Live, cmd)
	}
}

func (bpl *BiliPushConfigList) PushOnline(pkt *gjson.Result) {
	liveTime := time.Now()
	if lt := pkt.Get("live_time").Int(); lt != 0 {
		liveTime = time.Unix(lt, 0) // almost equivalent to time.Now()
	}
	if !biliPushHistoryLive.Online(bpl.Live, liveTime) {
		return
	}

	live, err := biligo.FetchLiveStatus(Itoa(bpl.Uid))
	if err != nil {
		log.Error("[BiliPush] failed to fetch format live uid: ", err)
		return
	}
	msg := bpl.name + "开播了\n" + live[Itoa(bpl.Uid)].DoTemplate()
	PushMsg(msg, bpl.Users, bpl.Groups)
}

func (bpl *BiliPushConfigList) PushOffline(pkt *gjson.Result) {
	offTime := time.Now()
	if !biliPushHistoryLive.Offline(bpl.Live, offTime) {
		return
	}

	live, err := biligo.FetchLiveStatus(Itoa(bpl.Uid))
	if err != nil {
		log.Error("[BiliPush] failed to fetch format live uid: ", err)
		return
	}
	msg := bpl.name + "下播了\n" + live[Itoa(bpl.Uid)].DoTemplate()
	PushMsg(msg, bpl.Users, bpl.Groups)
}

func (bpl *BiliPushConfigList) PushChange(pkt *gjson.Result) {
	newName := pkt.Get("data.title").String()
	if newName == bpl.title {
		return
	}
	msg := fmt.Sprintf("%s 更新了直播间标题：\n%s\n-> %s", bpl.name, bpl.title, newName)
	bpl.title = newName // 推送后再更新
	PushMsg(msg, bpl.Users, bpl.Groups)
}

func (bpl *BiliPushConfigList) PushWarning(pkt *gjson.Result) {
	reason := pkt.Get("msg").String()
	if reason == "" {
		return
	}
	msg := fmt.Sprintf("%s 直播间被警告：\n%s", bpl.name, reason)
	PushMsg(msg, bpl.Users, bpl.Groups)
}

func (bpl *BiliPushConfigList) PushCutOff(pkt *gjson.Result) {
	reason := pkt.Get("msg").String()
	if reason == "" {
		return
	}
	msg := fmt.Sprintf("%s 直播间被切断：\n%s", bpl.name, reason)
	PushMsg(msg, bpl.Users, bpl.Groups)
}

var biliPushHistoryDynamic = BiliPushHistoryDynamic{}

type BiliPushHistoryDynamic struct {
	Length  int
	Index   int
	History []string
	mu      sync.Mutex
}

func (bph *BiliPushHistoryDynamic) Init(length int) {
	bph.Length = length
	bph.Index = 0
	bph.History = make([]string, length)
}

func (bph *BiliPushHistoryDynamic) Add(idStr string) {
	bph.mu.Lock()
	defer bph.mu.Unlock()
	bph.History[bph.Index] = idStr
	bph.Index = (bph.Index + 1) % bph.Length
}

func (bph *BiliPushHistoryDynamic) Query(idStr string) bool {
	return slices.Contains(bph.History, idStr)
}

var biliPushHistoryLive = BiliPushHistoryLive{
	m: make(map[int]*BiliPushLiveState),
}

type (
	BiliPushLiveState struct {
		LastPushOnline  time.Time
		LastPushOffline time.Time
	}
	BiliPushHistoryLive struct {
		m  map[int]*BiliPushLiveState // roomId
		mu sync.Mutex
	}
)

func (bph *BiliPushHistoryLive) Online(roomId int, liveTime time.Time) (pass bool) {
	bph.mu.Lock()
	defer bph.mu.Unlock()

	state, ok := bph.m[roomId]
	defer func() {
		if pass {
			state.LastPushOnline = liveTime
		}
	}()

	pass = true
	switch {
	case !ok:
		// 没有状态, 直接推送
		state = &BiliPushLiveState{}
		bph.m[roomId] = state

	case state.LastPushOffline.After(state.LastPushOnline):
		// 之前状态为下播

	case liveTime.Sub(state.LastPushOnline) > biliPushConfig.liveMinimumInterval:
		// 间隔大于最小间隔

	default:
		pass = false
	}
	return
}

func (bph *BiliPushHistoryLive) Offline(roomId int, offTime time.Time) (pass bool) {
	bph.mu.Lock()
	defer bph.mu.Unlock()

	state, ok := bph.m[roomId]
	defer func() {
		if pass {
			state.LastPushOffline = offTime
		}
	}()

	pass = true
	switch {
	case !ok:
		// 没有状态, 直接推送
		state = &BiliPushLiveState{}
		bph.m[roomId] = state

	case state.LastPushOnline.After(state.LastPushOffline):
		// 之前状态为开播

	case offTime.Sub(state.LastPushOffline) > biliPushConfig.liveMinimumInterval:
		// 间隔大于最小间隔

	default:
		pass = false
	}
	return
}
