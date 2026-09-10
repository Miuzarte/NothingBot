package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"NothingBot_v4/utils"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/Miuzarte/biligo"
	"github.com/redis/rueidis"
)

// 本文件的日志 scope
var logBiliParse = logger.New("BiliParse")

type BiliParseConfig struct {
	SameParseInterval string // 同一会话重复解析同一链接的间隔 (秒)
	sameParseInterval time.Duration

	List
}

func (bpc *BiliParseConfig) Pass(ctx *EasyOnebot.Ctx) (pass bool) {
	return bpc.Black(ctx)
}

const biliParseMId ModuleId = "BiliParse"

var biliParseConfig = BiliParseConfig{}

var moduleBiliParse = Module{
	ModuleMeta: ModuleMeta{
		Name:       biliParseMId,
		Desc:       "Bilibili链接解析",
		Conditions: Conditions{},
		HelpMsg: "参数：" +
			"\n--download 下载并发送 (也可以在回复一条B站链接消息时带上这个参数)",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_BiliParse.go")

	moduleBiliParse.Init = initBiliParse
	modules.Add(&moduleBiliParse)
}

func initBiliParse() {
	err := config.DecodeModule(biliParseMId, &biliParseConfig)
	if err != nil {
		logBiliParse.Error().
			Err(err).
			Msg("failed to decode config")
		return
	}

	biliParseConfig.sameParseInterval, _ = time.ParseDuration(biliParseConfig.SameParseInterval)
	if biliParseConfig.sameParseInterval <= 0 {
		biliParseConfig.sameParseInterval = time.Minute
	}

	onebot.AddMatcher(moduleBiliParse.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnFunc(biliParseConfig.Pass).
		IsNotForwardMsg().
		Do(moduleBiliParse.RWMuWrap(ctxBiliParse)),
	)
	onebot.AddMatcher(biliParseMId.WithSuffix("track").String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnFunc(biliParseConfig.Pass).
		OnReply().
		Do(moduleBiliParse.RWMuWrap(ctxBiliParseTrack)),
	)
}

const BILIBILI_AV_BV_REGEXP = `(?:BV1[1-9A-HJ-NP-Za-km-z]{9}|av[0-9]+)`

var biliAvBvReg = regexp.MustCompile(BILIBILI_AV_BV_REGEXP)

func ctxBiliParse(ctx *EasyOnebot.Ctx) {
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pDownload := strings.Contains(lowerRm, "--download") || strings.Contains(lowerRm, "--dl")
	pForce := strings.Contains(lowerRm, "--force") && ctx.IsSuperuser
	pNeedAck := false // 识别 av/bv 号需要确认解析

	var results []biligo.ParseResult
	msg := ctx.UnescapedMessage
	useReplyedMsg := false
AGAIN:
	results, _ = biligo.ParseLink(msg)
	if len(results) == 0 {
		// 没结果时尝试直接匹配 av/bv 号
		textSegs := ctx.ParsedSegments.GetType(message.TYPE_TEXT) // 只匹配文本
		ids := biliAvBvReg.FindAllString(textSegs.String(), -1)

		if len(ids) != 0 {
			// av/bv 号结果
			results = biliAvBvToResults(ids)
			pNeedAck = true

		} else if ctx.ReplyId != 0 && !useReplyedMsg {
			// 尝试获取回复的消息
			replyed, err := ctx.GetReplyMsg()
			if err != nil {
				logBiliParse.Warn().
					Err(err).
					Msg("failed to get reply msg")
				return
			}
			// 使用回复的消息重新解析
			msg = replyed.Message.String()
			useReplyedMsg = true
			goto AGAIN
		}
	}

	if len(results) == 0 {
		return
	}

	if pDownload {
		videoResults := make([]biligo.ParseResult, 0, len(results))
		for _, result := range results {
			if result.Type == biligo.LINK_TYPE_ARCHIVE {
				videoResults = append(videoResults, result)
			}
		}
		switch len(videoResults) {
		case 0:
			ctx.SendMsgReply("没有找到视频链接！")
		case 1:
			go biliDownloadAndSend(ctx, videoResults[0], pForce)
		default:
			if !pForce {
				ctx.SendMsgReply("不允许同时下载多个视频！")
			} else {
				for _, result := range videoResults {
					go biliDownloadAndSend(ctx, result, pForce)
				}
			}
		}
	}

	if useReplyedMsg {
		return // 不解析回复, 只执行下载
	}

	if !pNeedAck {
		biliParseAndSend(ctx, biliParseCleanResults(ctx.Event.GroupId, results))
	} else {
		biliParseWaitAck(ctx, results)
	}
}

func ctxBiliParseTrack(ctx *EasyOnebot.Ctx) {
	if ctx.ReplyId == 0 {
		logBiliParse.Warn().
			Int("messageId", ctx.Event.MessageId).
			Msg("reply id is 0")
		return
	}
	origMsgId, err := biliParseTrackGet(ctx.Event.GroupId, ctx.ReplyId)
	if err != nil {
		if !rueidis.IsRedisNil(err) {
			logBiliParse.Warn().
				Err(err).
				Int("replyId", ctx.ReplyId).
				Int("group", ctx.Event.GroupId).
				Msg("failed to get track")
		}
		return
	}
	segs := make(message.SegmentArray, len(ctx.ParsedSegments))
	copy(segs, ctx.ParsedSegments)
	segs.DeleteAt(ctx.Event.SelfId) // 删掉 at 自身的消息段
	segChain := make(message.SegmentArray, 0, len(segs)+1)
	segChain.Append(message.Reply(origMsgId))
	segChain.Append(segs...)
	ctx.SendMsg(segChain)
}

func biliAvBvToResults(ids []string) (results []biligo.ParseResult) {
	for i, id := range ids {
		aid, err := biligo.AnyToAid(id)
		if err != nil {
			logBiliParse.Warn().
				Err(err).
				Msg("failed to convert av/bv to aid")
			continue
		}
		ids[i] = aid
	}
	set := utils.Set[string]{}
	ids = set.Clean(ids)
	results = make([]biligo.ParseResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, biligo.ParseResult{
			Type:    biligo.LINK_TYPE_ARCHIVE,
			Content: id,
		})
	}
	return results
}

type biliParseAckMapKey struct {
	groupId int
	userId  int
}

var (
	// groupId:userId -> cancel, 有协程会删除 matcher, 只需要保存 cancel
	biliParseAckMap = make(map[biliParseAckMapKey]context.CancelFunc)
	bpakMu          sync.Mutex
)

// biliParseWaitAck 匹配到 av/bv 号时等待确认解析,
func biliParseWaitAck(ctx *EasyOnebot.Ctx, results []biligo.ParseResult) {
	if ctx.Event.MessageType == event.TYPE_L2_MESSAGE_PRIVATE {
		// 私聊直接发
		biliParseAndSend(ctx, results)
		return
	}

	if ctx.Event.GroupId == 0 {
		logBiliParse.Warn().Msg("groupId is 0")
		return
	}

	resp, err := ctx.SendMsg("识别到 av/bv 号，是否解析？（y/n）")
	if err != nil {
		logBiliParse.Warn().
			Err(err).
			Msg("failed to send msg")
		return // 提示消息没发出去
	}

	// 群聊注册 matcher
	bpakMu.Lock()
	defer bpakMu.Unlock()
	mapKey := biliParseAckMapKey{ctx.Event.GroupId, ctx.Event.UserId}
	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	if cancel, ok := biliParseAckMap[mapKey]; ok {
		cancel() // 取消之前的操作
	}
	biliParseAckMap[mapKey] = cancel

	matcherKey := fmt.Sprintf("%s_tmp:%d:%d:%d", moduleBiliParse.Name, ctx.Event.GroupId, ctx.Event.UserId, ctx.Event.MessageId)
	ctx.RegisterTempMatcher(tctx, matcherKey,
		func(c *EasyOnebot.Ctx) {
			// 去除 reply 之类的 seg
			textSegs := c.ParsedSegments.GetType(message.TYPE_TEXT)
			text := strings.TrimSpace(textSegs.String())
			if strings.Contains(text, "--raw") {
				c.SendMsgReply(fmt.Sprint(results))
			}
			b, ok := utils.ParseBool(text)
			if !ok {
				return
			}
			cancel()
			if b {
				biliParseAndSend(ctx, results) // 需要手动确认, 不做 clean
			}
		},
		func() {
			ctx.DeleteMsg(resp.MessageID) // 撤回提示
			if bpakMu.TryLock() {         // 失败时正在写入新 matcher, 不需要删除
				delete(biliParseAckMap, mapKey)
				bpakMu.Unlock()
			}
		},
	)
}

func biliParseAndSend(ctx *EasyOnebot.Ctx, results []biligo.ParseResult) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	replys := make([]string, 0, len(results))

	linkTypeSum := [9]int{}

	var topComments map[string]*biligo.ReplyList       // 视频对应的置顶评论
	var descs map[string]string                        // 视频对应的简介
	var conclusions map[string]*biligo.VideoConclusion // 视频对应的总结

	var mediaSection *biligo.MediaSection // 番剧对应的分集, 单解析时发送

	var spaceDynamics *biligo.DynamicSpace // 用户空间动态, 单解析时发送

	// 统计各类型数量
	for i := range results {
		linkTypeSum[results[i].Type]++
	}

	// 获取并格式化
	for _, result := range results {
		switch result.Type {
		case biligo.LINK_TYPE_ARCHIVE: // 视频解析, 分开内容与总结、置顶评论
			video, _, conclusion, err := biligo.FetchVideoInfo3(result.Content)
			if err != nil {
				replys = append(replys, fmt.Sprint("failed to fetch video: ", err))
				continue
			}
			if len(results) == 1 {
				rl, err := video.GetReplyList()
				if err == nil && rl.Upper.Top.Content.Message != "" {
					if topComments == nil {
						topComments = make(map[string]*biligo.ReplyList)
					}
					topComments[result.Content] = &rl
				}
			}
			replys = append(replys, video.DoTemplate())
			if len(video.Desc) > 0 && IsAlphanumeric(video.Desc[0]) {
				if descs == nil {
					descs = make(map[string]string)
				}
				descs[result.Content] = video.Desc
			}
			if conclusion.Ok() {
				if conclusions == nil {
					conclusions = make(map[string]*biligo.VideoConclusion)
				}
				conclusions[result.Content] = &conclusion
			}

		case biligo.LINK_TYPE_MEDIA: // 番剧解析
			// 在单条解析时, 再发送分集
			id := result.Content
			var ssid string // for sections fetching

			switch id[:2] {
			case "md":
				mb, err := biligo.FetchMediaInfoBase(id)
				if err != nil {
					replys = append(replys, fmt.Sprint("failed to fetch media: ", err))
					continue
				}
				if mb.Media.SeasonId == 0 {
					replys = append(replys, fmt.Sprint("failed to fetch media: ", mb))
					continue
				}
				id = "ss" + Itoa(mb.Media.SeasonId)
				fallthrough

			case "ss":
				m, err := biligo.FetchMediaInfoSsid(id)
				if err != nil {
					replys = append(replys, fmt.Sprint("failed to fetch media: ", err))
					continue
				}
				ssid = id
				replys = append(replys, m.DoTemplate())

			case "ep":
				m, err := biligo.FetchMediaInfoEpid(id)
				if err != nil {
					replys = append(replys, fmt.Sprint("failed to fetch media: ", err))
					continue
				}
				ssid = Itoa(m.SeasonId)
				replys = append(replys, m.DoTemplate())

			default:
				panic("unreachable, fix regexp")
			}

			// 仅在单条解析时发送
			if linkTypeSum[biligo.LINK_TYPE_MEDIA] == 1 {
				ms, err := biligo.FetchMediaSection(ssid)
				if err != nil {
					logBiliParse.Error().
						Err(err).
						Msg("failed to fetch media section")
				} else {
					mediaSection = &ms
				}
			}

		case biligo.LINK_TYPE_SPACE: // 用户空间解析
			// 在单条解析时, 除了发送卡片以外, 再发送一页用户动态
			spaceInfo, err := result.FetchFormat()
			if err != nil {
				replys = append(replys, fmt.Sprint("failed to fetch format space: ", err))
				continue
			}
			replys = append(replys, spaceInfo)

			// 仅在单条解析时发送
			if linkTypeSum[biligo.LINK_TYPE_SPACE] == 1 {
				sd, err := biligo.FetchDynamicSpaceFix(result.Content)
				if err != nil {
					logBiliParse.Error().
						Err(err).
						Msg("failed to fetch dynamic space")
				} else if len(sd.Items) != 0 {
					spaceDynamics = &sd
				}
			}

		default:
			s, err := result.FetchFormat()
			if err != nil {
				replys = append(replys, fmt.Sprint("failed to fetch format: ", err))
				continue
			}
			replys = append(replys, s)
		}
	}

	var sentMsgId int64

	if len(replys) > 1 { // 多条用合并转发
		forward := message.SegmentArray{}
		for i, reply := range replys {
			segChain := message.ParseCqCodes(reply)

			if descs != nil {
				if d, cOk := descs[results[i].Content]; cOk {
					segChain.Append(
						message.Text("\n\n简介：\n"),
						message.Text(d),
					)
				}
			}
			if topComments != nil {
				if tc, ok := topComments[results[i].Content]; ok {
					segChain.Append(
						message.Text(
							"\n\n[置顶]" + tc.Upper.Top.Member.Uname + "：\n" +
								tc.Upper.Top.Content.Message,
						),
					)
				}
			}
			if conclusions != nil {
				if c, ok := conclusions[results[i].Content]; ok {
					segChain.Append(
						message.Text("\n\nBilibili AI总结：\n"),
						message.Text(c.DoTemplate()),
					)
				}
			}

			// 无总结不注册轮询
			// 不发送分集

			forward.Append(
				message.Node3(uid, name, segChain),
			)
		}

		resp, err := ctx.SendForwardMsgAuto(forward)
		if err != nil {
			logBiliParse.Error().
				Err(err).
				Msg("failed to send forward msg")
			return
		}
		sentMsgId = resp.MessageID

	} else { // 单条解析
		resp, err := ctx.SendMsg(replys[0])
		if err != nil {
			logBiliParse.Warn().
				Err(err).
				Msg("failed to send msg")
			return
		}
		sentMsgId = resp.MessageID

		result := results[0]

		// 番剧发送分集
		if mediaSection != nil {
			forward := make(message.SegmentArray, 0, len(mediaSection.MainSection.Episodes))

			for _, s := range mediaSection.MainSection.Episodes {
				forward.Append(
					message.Node3(uid, name, message.SegmentArray{
						message.Image(s.Cover),
						message.Text("\n" + s.LongTitle),
						message.Text("\nbilibili.com/bangumi/play/ep" + Itoa(s.Id)),
					}),
				)
			}

			_, err := ctx.SendForwardMsgAuto(forward)
			if err != nil {
				logBiliParse.Warn().
					Err(err).
					Msg("failed to send forward msg")
			}
		}

		// 用户空间发送动态
		if spaceDynamics != nil {
			forward := make(message.SegmentArray, 0, len(spaceDynamics.Items))

			for _, item := range spaceDynamics.Items {
				forward.Append(
					message.Node3(uid, name, message.ParseCqCodes(item.DoTemplate())),
				)
			}

			_, err := ctx.SendForwardMsgAuto(forward)
			if err != nil {
				logBiliParse.Warn().
					Err(err).
					Msg("failed to send forward msg")
			}
		}

		// 视频发送简介、置顶评论、总结
		// 发不发取决于是否有总结
		// 另外以合并转发形式发送
		if result.Type == biligo.LINK_TYPE_ARCHIVE {
			buildConclusionMsgF := func(desc string, topComment *biligo.ReplyList, conclusion *biligo.VideoConclusion) (forward message.SegmentArray) {
				if desc != "" {
					forward.Append(
						message.Node3(uid, name, message.SegmentArray{
							message.Text("简介：\n" + desc),
						}),
					)
				}
				if topComment != nil {
					forward.Append(
						message.Node3(uid, name, message.SegmentArray{
							message.Text(
								"[置顶] " + topComment.Upper.Top.Member.Uname + "：\n" +
									topComment.Upper.Top.Content.Message,
							),
						}),
					)
				}
				if conclusion != nil {
					forward.Append(
						message.Node3(uid, name, message.SegmentArray{
							message.Text("Bilibili AI总结：\n" + conclusion.DoTemplate()),
						}),
					)
				}
				return forward
			}

			if conclusions != nil {
				if vc, ok := conclusions[result.Content]; ok {
					var desc string
					if descs != nil {
						desc = descs[result.Content]
					}
					var topComment *biligo.ReplyList
					if topComments != nil {
						topComment = topComments[result.Content]
					}

					_, err := ctx.SendForwardMsgAuto(buildConclusionMsgF(desc, topComment, vc))
					if err != nil {
						logBiliParse.Warn().
							Err(err).
							Msg("failed to send forward msg")
					}
				}
			} else {
				// 没有总结时注册轮询
				logBiliParse.Debug().
					Str("content", result.Content).
					Msg("register video conclusion")
				tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				biligo.RegisterVideoConclusion(tctx, result.Content, "", func(vc biligo.VideoConclusion, err error) {
					cancel()
					if err != nil {
						if !biligo.UnwrapErr(err).Is(biligo.ErrPollNoSummary) {
							logBiliParse.Warn().
								Err(err).
								Msg("failed to poll video conclusion")
						}
						return
					}

					var desc string
					if descs != nil {
						desc = descs[result.Content]
					}
					var topComment *biligo.ReplyList
					if topComments != nil {
						topComment = topComments[result.Content]
					}

					_, err = ctx.SendForwardMsgAuto(buildConclusionMsgF(desc, topComment, &vc))
					if err != nil {
						logBiliParse.Warn().
							Err(err).
							Msg("failed to send forward msg")
						return
					}
				})

			}
		}
	}

	biliParseTrackSet(ctx.Event.GroupId, sentMsgId, int64(ctx.Event.MessageId))
}

func biliDownloadAndSend(ctx *EasyOnebot.Ctx, result biligo.ParseResult, pForce bool) {
	if result.Type != biligo.LINK_TYPE_ARCHIVE {
		msg := fmt.Sprintf("occured a non-archive link: %s", result.Content)
		logBiliParse.Error().Msg(msg)
		onebot.Log2Sus.Error(msg)
		return
	}

	c, cancel := context.WithCancel(context.Background())
	defer cancel()

	qn := biligo.VIDEO_QN_720
	var qnStr string
AGAIN:
	switch qn {
	case biligo.VIDEO_QN_720:
		qnStr = "720P"
	case biligo.VIDEO_QN_360:
		qnStr = "360P"
	default:
		logBiliParse.Panic().
			Int("qn", qn).
			Msg("unreachable case")
	}

	vd := biligo.NewDownloadVideoMp4(c, result.Content, "", qn)
	size, err := vd.Init()
	if err != nil {
		ctx.SendMsgf("(%s)(%s)获取失败：%v", result.Content, qnStr, err)
		return
	}

	if !pForce && size > 64*1024*1024 { // 64MiB
		if qn != biligo.VIDEO_QN_360 { // try 360P
			qn = biligo.VIDEO_QN_360
			goto AGAIN
		}
		ctx.SendMsgf("(%s)(%s)太大啦(%s > 64MiB)，去网页看吧", result.Content, qnStr, utils.FormatBytes(uint64(size)))
		return
	}

	dlResp, err := ctx.SendMsgf("(%s)(%s)下载中(%s)...", result.Content, qnStr, utils.FormatBytes(uint64(size)))
	if err != nil {
		logBiliParse.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	bb := utils.Base64Builder(int(size), true)
	defer bb.Close()

	n, err := vd.Start(bb)
	ctx.DeleteMsg(dlResp.MessageID)
	if err != nil {
		ctx.SendMsgf("(%s)(%s)下载失败：%v", result.Content, qnStr, err)
		return
	}
	if n != int64(size) {
		ctx.SendMsgf("(%s)(%s)下载出错：size != n(%d)", result.Content, qnStr, n)
	}

	// respSend, err := ctx.SendMsgf("发送中...\n已为该视频播放量+1，请放心食用")
	respSend, err := ctx.SendMsgf("(%s)(%s)发送中...", result.Content, qnStr)
	if err != nil {
		logBiliParse.Error().
			Err(err).
			Msg("failed to send msg")
	}
	_, err = ctx.SendMsg(message.Video(bb.String()))
	ctx.DeleteMsg(respSend.MessageID)
	if err != nil {
		ctx.SendMsgf("(%s)(%s)发送失败：%v", result.Content, qnStr, err)
		return
	}
}

type BiliParseHistory struct {
	biligo.ParseResult
	Time time.Time
}

func biliParseHistorySet(group int, content string, ph *BiliParseHistory) error {
	key := "bili_parse_history:" + Itoa(group) + ":" + content
	return redisClient.Set(key, ph, biliParseConfig.sameParseInterval)
}

func biliParseHistoryGet(group int, content string) (*BiliParseHistory, error) {
	key := "bili_parse_history:" + Itoa(group) + ":" + content
	return UnmarshalResult[BiliParseHistory](redisClient.Get(key))
}

func biliParseCleanResults(groupId int, results []biligo.ParseResult) []biligo.ParseResult {
	if groupId == 0 { // 私聊不处理
		return results
	}
	tn := time.Now()
	cleaned := make([]biligo.ParseResult, 0, len(results))
	for _, r := range results {
		bph, err := biliParseHistoryGet(groupId, r.Content)
		if err != nil {
			if !rueidis.IsRedisNil(err) {
				logBiliParse.Error().
					Err(err).
					Msg("failed to get bili parse history")
			}
			cleaned = append(cleaned, r) // 出错时不过滤
			continue
		}
		if bph != nil &&
			tn.Sub(bph.Time) < biliParseConfig.sameParseInterval {
			continue
		}
		err = biliParseHistorySet(groupId, r.Content, &BiliParseHistory{r, tn})
		if err != nil {
			logBiliParse.Error().
				Err(err).
				Msg("failed to set bili parse history")
		}
		cleaned = append(cleaned, r)
	}
	return cleaned
}

func biliParseTrackSet(group int, msgId, origMsgId int64) error {
	key := "bili_parse_track:" + Itoa(group) + ":" + Itoa(msgId)
	return redisClient.Set(key, Itoa(origMsgId), 24*time.Hour)
}

func biliParseTrackGet(group, msgId int) (origMsgId int, err error) {
	key := "bili_parse_track:" + Itoa(group) + ":" + Itoa(msgId)
	r := redisClient.Get(key)
	if r.Err != nil {
		return 0, r.Err
	}
	if r.Data == "" {
		return 0, nil
	}
	return strconv.Atoi(r.Data)
}
