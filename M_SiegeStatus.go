package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"regexp"
	"slices"
	"strings"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
	"github.com/Miuzarte/SiegeStatus"
	"github.com/redis/rueidis"
)

// 本文件的日志 scope
var logSiegeStatus = logger.New("SiegeStatus")

const SIEGE_STATUS_REGEXP = `(?i)(当前)?(彩虹?六号?|围攻)服务器状态|/siegeStatus|rainbow-six/siege/status`

var siegeStatusReg = regexp.MustCompile(SIEGE_STATUS_REGEXP)

type SiegeStatusConfig struct {
	PollingInterval string
	pollingInterval time.Duration

	Groups []int
	Users  []int

	QueryIds []string
	queryIds []SiegeStatus.AppId

	ctx     context.Context
	cancel  context.CancelFunc
	ticker  *time.Ticker
	trigger chan *EasyOnebot.Ctx
}

var siegeStatusQueryIdsMap = map[string]SiegeStatus.AppId{
	"pc":      SiegeStatus.APP_ID_SIEGE_PC,
	"windows": SiegeStatus.APP_ID_SIEGE_PC,

	"ps4":   SiegeStatus.APP_ID_SIEGE_ORBIS,
	"orbis": SiegeStatus.APP_ID_SIEGE_ORBIS,

	"ps5": SiegeStatus.APP_ID_SIEGE_PS5,

	"xboxxs": SiegeStatus.APP_ID_SIEGE_SCARLETT,

	"xboxone": SiegeStatus.APP_ID_SIEGE_DURANGO,
	"xbox1":   SiegeStatus.APP_ID_SIEGE_DURANGO,
}

const siegeStatusMId ModuleId = "SiegeStatus"

var (
	siegeStatusConfig     *SiegeStatusConfig
	siegeStatusRedisPurge bool
)

var moduleSiegeStatus = Module{
	ModuleMeta: ModuleMeta{
		Name:    siegeStatusMId,
		Desc:    "《彩虹六号：围攻》服务状态查看/变更推送",
		HelpMsg: "\"彩六服务器状态\" \"/siegeStatus\"",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_SiegeStatus.go")

	moduleSiegeStatus.Init = initSiegeStatus
	moduleSiegeStatus.ReInit = initSiegeStatus
	moduleSiegeStatus.AtExit = func() {
		if siegeStatusConfig != nil {
			siegeStatusConfig.Stop()
		}
	}
	modules.Add(&moduleSiegeStatus)
}

func initSiegeStatus() {
	newConfig := &SiegeStatusConfig{}
	err := config.DecodeModule(siegeStatusMId, newConfig)
	if err != nil {
		logSiegeStatus.Error().
			Err(err).
			Msg("failed to decode config")
		return
	}

	if siegeStatusConfig != nil {
		siegeStatusConfig.Stop()
	}

	oldConfig := siegeStatusConfig
	siegeStatusConfig = newConfig

	siegeStatusConfig.ctx, siegeStatusConfig.cancel = context.WithCancel(context.Background())
	siegeStatusConfig.trigger = make(chan *EasyOnebot.Ctx)

	siegeStatusConfig.pollingInterval, _ = time.ParseDuration(siegeStatusConfig.PollingInterval)
	if siegeStatusConfig.pollingInterval < time.Second*30 {
		logSiegeStatus.Warn().
			Dur("interval", siegeStatusConfig.pollingInterval).
			Msg("polling interval too short")
		siegeStatusConfig.pollingInterval = time.Minute
	}
	siegeStatusConfig.ticker = time.NewTicker(siegeStatusConfig.pollingInterval)

	siegeStatusConfig.queryIds = make([]SiegeStatus.AppId, 0, len(siegeStatusConfig.QueryIds))
	for _, queryId := range siegeStatusConfig.QueryIds {
		if len(queryId) == 36 { // raw uuid
			siegeStatusConfig.queryIds = append(siegeStatusConfig.queryIds, queryId)
		} else {
			id, ok := siegeStatusQueryIdsMap[strings.ToLower(queryId)]
			if !ok {
				logSiegeStatus.Warn().
					Str("type", queryId).
					Msg("unknown platform type")
				continue
			}
			siegeStatusConfig.queryIds = append(siegeStatusConfig.queryIds, id)
		}
	}
	slices.Sort(siegeStatusConfig.queryIds)

	siegeStatusRedisPurge = oldConfig == nil ||
		!slices.Equal(siegeStatusConfig.queryIds, oldConfig.queryIds)

	siegeStatusConfig.Start()

	onebot.AddMatcher(moduleSiegeStatus.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnRegexpMatchString(siegeStatusReg).
		Do(moduleSiegeStatus.RWMuWrap(ctxSiegeStatus)),
	)
}

func ctxSiegeStatus(ctx *EasyOnebot.Ctx) {
	siegeStatusConfig.trigger <- ctx
}

func (ssc *SiegeStatusConfig) Start() {
	go func() {
		for {
			select {
			case <-ssc.ctx.Done():
				return

			case ctx := <-ssc.trigger:
				// logSiegeStatus.Debug().Msg("ctx := <-ssc.trigger: go ssc.PushCurr(ctx)")
				go ssc.PushCurr(ctx)

			case <-ssc.ticker.C:
				// logSiegeStatus.Debug().Msg("<-ssc.ticker.C: go ssc.PushDiff()")
				go ssc.PushDiff()

			}
		}
	}()
}

func (ssc *SiegeStatusConfig) Stop() {
	ssc.cancel()
}

func (ssc *SiegeStatusConfig) PushCurr(ctx *EasyOnebot.Ctx) {
	resp, err := SiegeStatus.Get(ssc.ctx, ssc.queryIds...)
	if err != nil {
		logSiegeStatus.Warn().
			Err(err).
			Msg("failed to get siege status")
		return
	}

	msg := siegeStatusFormatStatus(&resp)
	if ctx != nil {
		_, err = ctx.SendMsg(msg)
	} else {
		err = PushMsg(msg, ssc.Users, ssc.Groups)
	}
	if err != nil {
		logSiegeStatus.Warn().
			Err(err).
			Msg("failed to push siege status curr")
		return
	}
}

func (ssc *SiegeStatusConfig) PushDiff() {
	new, err := SiegeStatus.Get(ssc.ctx, ssc.queryIds...)
	if err != nil {
		logSiegeStatus.Warn().
			Err(err).
			Msg("failed to get siege status")
		return
	}
	old := siegeStatusCacheRead()
	siegeStatusCacheWrite(new)

	msg := siegeStatusFormatStatusDiff(&old, &new)
	if msg == nil {
		return
	}

	err = PushMsg(msg, ssc.Users, ssc.Groups)
	if err != nil {
		logSiegeStatus.Warn().
			Err(err).
			Msg("failed to push siege status diff")
		return
	}
}

func siegeStatusFormatStatus(resp *SiegeStatus.Response) (msg message.SegmentArray) {
	msg = make(message.SegmentArray, 0, 1+len(resp.GameStatuses))
	msg.Append(message.Textf(
		"《彩虹六号：围攻》服务状态\nubisoft.com/zh-cn/game/rainbow-six/siege/status\n%s",
		resp.LastModifiedAt.Local(),
	))
	for i := range resp.GameStatuses {
		msg.Append(message.Text(
			"\n\n" + siegeStatusFormatGameStatus(nil, &resp.GameStatuses[i]),
		))
	}
	return
}

func siegeStatusFormatStatusDiff(old, new *SiegeStatus.Response) message.SegmentArray {
	msg := message.SegmentArray{}
	for i := range siegeStatusesDiffIter(old.GameStatuses, new.GameStatuses) {
		msg.Append(message.Text(
			"\n\n" + siegeStatusFormatGameStatus(&old.GameStatuses[i], &new.GameStatuses[i]),
		))
	}
	if len(msg) == 0 {
		return nil
	}
	return append(message.SegmentArray{message.Textf(
		"《彩虹六号：围攻》服务状态更新\nubisoft.com/zh-cn/game/rainbow-six/siege/status\n%s",
		new.LastModifiedAt.Local(),
	)}, msg...)
}

func siegeStatusFormatGameStatus(old, new *SiegeStatus.GameStatus) string {
	var (
		name             string
		status           string
		isMaintenance    string
		impactedFeatures string
	)

	switch {
	case old == nil && new == nil:
		logSiegeStatus.Panic().Msg("old == nil && new == nil")
		panic("unreachable")
	case new == nil:
		logSiegeStatus.Panic().Msg("new == nil")
		panic("unreachable")

	case old == nil:
		name = fmt.Sprintf("%q", new.Name)
		status = fmt.Sprintf("%q", new.Status)
		isMaintenance = fmt.Sprintf("%t", new.IsMaintenance)
		impactedFeatures = fmt.Sprintf("%q", new.ImpactedFeatures)

	default:
		name = fmt.Sprintf("%q", new.Name)
		if old.Status != new.Status {
			status = fmt.Sprintf("%q -> %q", old.Status, new.Status)
		} else {
			status = fmt.Sprintf("%q", new.Status)
		}
		if old.IsMaintenance != new.IsMaintenance {
			isMaintenance = fmt.Sprintf("%t -> %t", old.IsMaintenance, new.IsMaintenance)
		} else {
			isMaintenance = fmt.Sprintf("%t", new.IsMaintenance)
		}
		if !slices.Equal(old.ImpactedFeatures, new.ImpactedFeatures) {
			impactedFeatures = fmt.Sprintf("%q -> %q", old.ImpactedFeatures, new.ImpactedFeatures)
		} else {
			impactedFeatures = fmt.Sprintf("%q", new.ImpactedFeatures)
		}

	}

	return fmt.Sprintf(
		`%s：
"status": %s
"isMaintenance": %s
"impactedFeatures": %s`,
		name,
		status,
		isMaintenance,
		impactedFeatures,
	)
}

func siegeStatusesDiffIter(gs1, gs2 []SiegeStatus.GameStatus) iter.Seq[int] {
	return func(yield func(int) bool) {
		if len(gs1) != len(gs2) {
			// unimplemented
			logSiegeStatus.Warn().Msg("[TODO] len(gs1) != len(gs2)")
			return
		}

		cmpFunc := func(a, b SiegeStatus.GameStatus) int {
			return cmp.Compare(a.Name, b.Name)
		}
		slices.SortFunc(gs1, cmpFunc)
		slices.SortFunc(gs2, cmpFunc)

		for i := range len(gs1) {
			if !siegeStatusEqual(&gs1[i], &gs2[i]) {
				if !yield(i) {
					break
				}
			}
		}
	}
}

func siegeStatusEqual(g1, g2 *SiegeStatus.GameStatus) bool {
	if g1 == g2 {
		return true
	}
	if g1 == nil || g2 == nil {
		return false
	}

	if g1.ApplicationId != g2.ApplicationId {
		logSiegeStatus.Warn().
			Str("name1", g1.Name).
			Str("appId1", g1.ApplicationId).
			Str("name2", g2.Name).
			Str("appId2", g2.ApplicationId).
			Msg("weird status comparison")
		return false
	}
	if g1.Status != g2.Status {
		return false
	}
	if g1.IsMaintenance != g2.IsMaintenance {
		return false
	}
	if !slices.Equal(g1.ImpactedFeatures, g2.ImpactedFeatures) {
		return false
	}

	return true
}

const SIEGE_STATUS_REDIS_KEY = "siege_status"

func siegeStatusCacheWrite(resp SiegeStatus.Response) {
	const expire = time.Hour * 24 * 7

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	v, err := json.Marshal(resp)
	if err != nil {
		logSiegeStatus.Panic().
			Err(err).
			Msg("failed to marshal response")
		return
	}

	// logSiegeStatus.Debug().Msgf("write to redis: %q: %q", SIEGE_STATUS_REDIS_KEY, v)
	result := redisClient.Client.Do(ctx,
		redisClient.Client.B().Set().Key(SIEGE_STATUS_REDIS_KEY).Value(string(v)).Ex(expire).Build(),
	)
	if err := result.Error(); err != nil {
		logSiegeStatus.Error().
			Err(err).
			Msg("failed to write redis")
		return
	}
}

func siegeStatusCacheRead() (resp SiegeStatus.Response) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	result := redisClient.Client.Do(ctx, redisClient.Client.B().Get().Key(SIEGE_STATUS_REDIS_KEY).Build())
	if err := result.Error(); err != nil {
		if !rueidis.IsRedisNil(err) {
			logSiegeStatus.Error().
				Err(err).
				Msg("failed to query redis")
		}
		return
	}

	s, err := result.ToString()
	if err != nil {
		logSiegeStatus.Error().
			Any("result", result).
			Err(err).
			Msg("failed to convert redis result to string")
		return
	}

	err = json.Unmarshal([]byte(s), &resp)
	if err != nil {
		logSiegeStatus.Error().
			Str("result", s).
			Err(err).
			Msg("failed to unmarshal redis result")
		return
	}

	return
}
