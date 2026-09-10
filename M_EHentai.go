package main

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"
	"NothingBot_v4/slicesyntax"
	"NothingBot_v4/utils"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	eh "github.com/Miuzarte/EHentai-go"
)

// 本文件的日志 scope
var logEHentai = logger.New("EHentai")

const (
	// [1]: E / E- / EX
	// [2]: keyword
	EHENTAI_SEARCH_REGEXP = `(?i)(?:(E-?|EX)(?:H|HENTAI)?)搜索?[\s:：]*(.+)`

	// https://e-hentai.org/g/3138775/30b0285f9b
	// [1]: url self
	// [2]: slice syntax
	EHENTAI_GALLERY_URL_REGEXP = `(?s)((?:ex|e-)hentai\.org/g/[0-9]+/[0-9a-z]+/?)` +
		`\s*(\[(?:-?\d*:?)*\](?:\[(?:-?\d*:?)*\])*)?`

	// https://e-hentai.org/s/859299c9ef/3138775-7
	// https://e-hentai.org/s/0b2127ea05/3138775-8
	// [0]: url self
	// [1]: page num // unused
	EHENTAI_PAGE_URL_REGEXP = `(?s)(?:ex|e-)hentai\.org/s/[0-9a-z]+/[0-9]+-([0-9]+)/?`
)

var (
	eHentaiSearchReg     = regexp.MustCompile(EHENTAI_SEARCH_REGEXP)
	eHentaiGalleryUrlReg = regexp.MustCompile(EHENTAI_GALLERY_URL_REGEXP)
	eHentaiPageUrlReg    = regexp.MustCompile(EHENTAI_PAGE_URL_REGEXP)
)

const eHentaiMId ModuleId = "EHentai"

var (
	moduleEHentaiSearch = ModuleMeta{
		Name:       eHentaiMId.WithSuffix("Search"),
		Desc:       "E(x)Hentai搜索",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg:    EHENTAI_SEARCH_REGEXP,
	}
	moduleEHentaiGalleryParse = ModuleMeta{
		Name:       eHentaiMId.WithSuffix("GalleryParse"),
		Desc:       "E(x)Hentai画廊链接解析",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg: EHENTAI_GALLERY_URL_REGEXP +
			"\n\n参数：" +
			"\n链接尾随(一或多个) [n:m] / [n:] / [:m] 以指定下载的页码范围, 支持负索引" +
			"\n--pdf 合并为 Pdf 文件发送" +
			"\n--noupload 不上传 Pdf 至文件",
	}
	moduleEHentaiPageParse = ModuleMeta{
		Name:       eHentaiMId.WithSuffix("PageParse"),
		Desc:       "E(x)Hentai画廊页链接解析",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg:    EHENTAI_PAGE_URL_REGEXP,
	}
)

var moduleEHentai = Module{
	ModuleMeta: ModuleMeta{
		Name:   eHentaiMId,
		Hidden: true,
	},
	Priority: 1, // after [moduleManga]
	Disable:  env.Testing,
	SubModules: []*ModuleMeta{
		&moduleEHentaiSearch,
		&moduleEHentaiGalleryParse,
		&moduleEHentaiPageParse,
	},
}

func init() {
	NoBuildPrintFile("M_EHentai.go")

	moduleEHentai.Init = initEHentai
	moduleEHentai.ReInit = initEHentai
	modules.Add(&moduleEHentai)
}

func initEHentai() {
	eh.SetCookie(
		mangaConfig.EHentaiCookie.IpbMemberId,
		mangaConfig.EHentaiCookie.IpbPassHash,
		mangaConfig.EHentaiCookie.Igneous,
		mangaConfig.EHentaiCookie.Sk,
	)
	if mangaConfig.Threads > 0 {
		eh.SetThreads(mangaConfig.Threads)
	}
	eh.SetDomainFronting(mangaConfig.EHentaiUseDomainFronting)
	eh.SetUseEnvProxy(mangaConfig.UseEnvProxy)
	if mangaConfig.EHentaiUseEhTagDB {
		go func() {
			for range 3 {
				ts := time.Now()
				err := eh.InitEhTagDb()
				if err == nil {
					logEHentai.Info().
						Dur("cost", time.Since(ts)).
						Msg("tag db init done")
					break
				}
				logEHentai.Warn().
					Err(err).
					Msg("failed to init tag db")
				<-time.After(time.Minute)
			}
		}()
	} else {
		eh.FreeEhTagDb()
	}
	eh.SetAutoCacheEnabled(mangaConfig.EHentaiGalleryCache)
	eh.SetCacheDir(mangaConfig.EHentaiCacheDir)
	eh.RegisterIgneousUpdate(func(igneous string) {
		onebot.Log2Sus.Info("[EHentai] igneous updated: ", igneous)
	})

	onebot.AddMatcher(
		moduleEHentaiSearch.Name.String(), EasyOnebot.NewMatcher().
			OnTypeL1(event.TYPE_L1_MESSAGE).
			IsNotCardMsg().
			OnlyType(message.TYPE_TEXT).
			OnFunc(func(ctx *EasyOnebot.Ctx) bool {
				return mangaConfig.EHentaiEnabled && mangaConfig.White(ctx)
			}).
			OnRegexpFindAllStringSubmatch(eHentaiSearchReg).
			Do(moduleEHentai.RWMuWrap(ctxEHentaiSearch)),
	)
	onebot.AddMatcher(
		moduleEHentaiGalleryParse.Name.String(), EasyOnebot.NewMatcher().
			OnTypeL1(event.TYPE_L1_MESSAGE).
			IsNotCardMsg().
			OnlyType(message.TYPE_TEXT).
			OnFunc(func(ctx *EasyOnebot.Ctx) bool {
				return mangaConfig.EHentaiEnabled && mangaConfig.White(ctx)
			}).
			OnRegexpFindAllStringSubmatch(eHentaiGalleryUrlReg).
			Do(moduleEHentai.RWMuWrap(ctxEHentaiGalleryParse)),
	)
	onebot.AddMatcher(
		moduleEHentaiPageParse.Name.String(), EasyOnebot.NewMatcher().
			OnTypeL1(event.TYPE_L1_MESSAGE).
			IsNotCardMsg().
			OnlyType(message.TYPE_TEXT).
			OnFunc(func(ctx *EasyOnebot.Ctx) bool {
				return mangaConfig.EHentaiEnabled && mangaConfig.White(ctx)
			}).
			OnRegexpFindAllStringSubmatch(eHentaiPageUrlReg).
			Do(moduleEHentai.RWMuWrap(ctxEHentaiPageParse)),
	)
}

func ctxEHentaiSearch(ctx *EasyOnebot.Ctx) {
	submatch := ctx.Submatches.Get(moduleEHentaiSearch.Name.String())[0]
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))
	pAll := strings.Contains(lowerRm, "--all")

	var site MangaSearchSite
	switch strings.ToLower(submatch[1]) {
	case "e", "e-":
		site = MANGA_SITE_EH
	case "ex":
		site = MANGA_SITE_EXH
	default:
		ctx.SendMsgReplyf("[EHentai] [FIXME] unexpected search site: %s", submatch[1])
		return
	}
	keyword := submatch[2]

	respSearch, err := ctx.SendMsg("[EHentai] 搜索中...")
	if err != nil {
		logEHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	ms := NewMangaSearch(ctx, tctx, site, keyword)
	ms.All = pAll
	forward, err := ms.Do()
	if err != nil {
		ctx.SendMsgf("[EHentai] 搜索失败：%v", err)
		if errors.Unwrap(err) == eh.ErrNoHitsFound {
			ctx.SendMsgf("[EHentai] 如果认为搜索关键词没问题，可再次尝试使用 --all 参数来搜索所有分类")
		}
		return
	}

	ts := time.Now()
	respSendForward, err := ctx.SendForwardMsgAuto(forward)
	ctx.Std.DeleteMsg(respSearch.MessageId)
	if err != nil {
		ctx.SendMsg("[EHentai] 搜索结果发送失败")
		return
	}
	respRecallHint, _ := ctx.SendMsgReplyf("[EHentai] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

	if !pNoRecall {
		registerTimerRecall(respSendForward.MessageId)
		if respRecallHint != nil {
			registerTimerRecall(respRecallHint.MessageId)
		}
	}
}

func ctxEHentaiGalleryParse(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatches := ctx.Submatches.Get(moduleEHentaiGalleryParse.Name.String())
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pUsePdf := strings.Contains(lowerRm, "--pdf")
	pNoUpload := strings.Contains(lowerRm, "--noupload") || strings.Contains(lowerRm, "--no-upload")
	pPurge := strings.Contains(lowerRm, "--purge")
	pDownload := strings.Contains(lowerRm, "--download") // 仅下载
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))
	pAll := strings.Contains(lowerRm, "--all") && ctx.IsSuperuser

	galleryUrls := make([]string, len(submatches))
	sss := make([]slicesyntax.SliceSyntaxes, len(submatches))
	for i, submatch := range submatches {
		galleryUrls[i] = submatch[1]
		if submatch[2] != "" {
			sss[i] = slicesyntax.ParseMulti(submatch[2])
		}
	}

	if len(galleryUrls) > 2 && !ctx.IsSuperuser {
		ctx.SendMsgf("[EHentai] ☝️哒咩！%d个画廊太多了", len(galleryUrls))
		return
	}

	respFetch, err := ctx.SendMsg("[EHentai] 获取中...")
	if err != nil {
		logEHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	epOp := EHentaiParseOption{
		DlAll:        pAll,
		ToPdf:        pUsePdf,
		Purge:        pPurge,
		DownloadOnly: pDownload,
	}
	if epOp.ToPdf {
		epOp.ToJpg = true
		epOp.Salt = false
	} else {
		epOp.ToJpg = false
		epOp.Salt = true
	}
	ep, err := NewEhGalleryParse(tctx, galleryUrls, sss, epOp)
	if err != nil {
		ctx.SendMsgReplyf("[EHentai] 解析失败：%v", err)
		return
	}

	failedGIds := eHentaiLock.TryLock(ep.GIds)
	if len(failedGIds) > 0 {
		ctx.SendMsgReplyf("[EHentai] locks of gallery(s) %v are holding by other users", failedGIds)
		return
	}
	defer eHentaiLock.Unlock(ep.GIds)

	// 每个画廊发一个合并转发
	for i, gId := range ep.GIds {
		respDownload, err := ctx.SendMsg(ep.DownloadingHint(i))
		if i == 0 {
			ctx.Std.DeleteMsg(respFetch.MessageId)
		}
		if err != nil {
			logEHentai.Error().
				Err(err).
				Msg("failed to send msg")
			return
		}

		// 使用 pdf 时也用合并转发发送画廊信息
		forward := message.SegmentArray{
			message.Node3(uid, name, ep.GalleryHeader(i)),
		}

		if !pUsePdf {
			n, nodes, err := ep.BuildForward(i, uid, name)
			if err != nil {
				ctx.SendMsgf("[EHentai] %v", err)
				return
			}

			if pDownload {
				_, err := ctx.SendMsgReplyf("[EHentai] 画廊 %d 下载完成 (%d)", gId, n)
				if err != nil {
					logEHentai.Error().
						Err(err).
						Msg("failed to send msg")
				}
				continue
			}

			respSend, err := ctx.SendMsg(ep.SendingHint(i))
			ctx.Std.DeleteMsg(respDownload.MessageId)
			if err != nil {
				logEHentai.Error().
					Err(err).
					Msg("failed to send msg")
				return
			}

			ts := time.Now()
			respSendForward, err := ctx.SendForwardMsgAuto(append(forward, nodes...))
			ctx.Std.DeleteMsg(respSend.MessageId)
			if err != nil {
				ctx.SendMsgf("[EHentai] 画廊 %d 发送失败", gId)
				return
			}
			respRecallHint, _ := ctx.SendMsgReplyf("[EHentai] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

			if !pNoRecall {
				registerTimerRecall(respSendForward.MessageId)
				if respRecallHint != nil {
					registerTimerRecall(respRecallHint.MessageId)
				}
			}

		} else {
			filename := fmt.Sprintf("%d.pdf", gId)
			filepath := filepath.Join(env.WorkDir, mangaConfig.EHentaiCacheDir, filename)
			_, err := ep.BuildPdf(i, filename, filepath)
			if err != nil {
				ctx.SendMsgf("[EHentai] %v", err)
				return
			}

			// 发送画廊信息
			_, err = ctx.SendForwardMsgAuto(forward)
			if err != nil {
				logEHentai.Error().
					Err(err).
					Int("gId", gId).
					Msg("failed to send msg")
				ctx.SendMsgf("[EHentai] 画廊 %d 信息合并转发发送失败", gId)
			}

			// 发送直链
			_, err = ctx.SendMsgf("[EHentai] 直接查看：https://ehentai.miuzarte.top/%s", filename)
			if err != nil {
				logEHentai.Error().
					Err(err).
					Int("gId", gId).
					Msg("failed to send msg")
				ctx.SendMsgf("[EHentai] 画廊 %d 直链发送失败", gId)
			}

			// 发送文件
			if !pNoUpload {
				ts := time.Now()
				switch ctx.Event.MessageType {
				case event.TYPE_L2_MESSAGE_GROUP:
					// [TODO] move to a folder
					err = ctx.UploadGroupFile(filepath, filename, "/")
				case event.TYPE_L2_MESSAGE_PRIVATE:
					err = ctx.UploadPrivateFile(filepath, filename)
				default:
					logEHentai.Warn().
						Str("type", ctx.Event.MessageType).
						Msg("unsupported message type")
					ctx.SendMsgf("[EHentai] 不支持的消息类型：%s", ctx.Event.MessageType)
					return
				}
				if err != nil {
					logEHentai.Error().
						Err(err).
						Int("gId", gId).
						Msg("failed to upload pdf")
					ctx.SendMsgf("[EHentai] 画廊 %d pdf上传失败：%v", gId, err)
					return
				}
				ctx.SendMsgReplyf("[EHentai] 画廊 %d pdf上传完毕！\n(%s)", gId, time.Since(ts))
			}

		}

	}
}

func ctxEHentaiPageParse(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatches := ctx.Submatches.Get(moduleEHentaiPageParse.Name.String())
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pUsePdf := strings.Contains(lowerRm, "--pdf")
	pDownload := strings.Contains(lowerRm, "--download") // 仅下载
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))

	if pUsePdf {
		ctx.SendMsgReply("[EHentai] pdf is not supported for pages link")
		return
	}

	if len(submatches) > mangaConfig.MaxForwardImages && !ctx.IsSuperuser {
		ctx.SendMsgf("[EHentai] ☝️哒咩！%d页太多了", len(submatches))
		return
	}

	pageUrlSet := utils.Set[string]{}
	for _, submatch := range submatches {
		pageUrlSet.Add(submatch[0])
	}
	pageUrls := pageUrlSet.Get()
	slices.Sort(pageUrls)

	respFetching, err := ctx.SendMsg("[EHentai] 获取中...")
	if err != nil {
		logEHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	epOp := EHentaiParseOption{
		Salt:         true,
		ToJpg:        false,
		DownloadOnly: pDownload,
	}
	ep, err := NewEhPageParse(tctx, pageUrls, epOp)
	if err != nil {
		ctx.SendMsgReplyf("[EHentai] 解析失败：%v", err)
		return
	}

	failedGIds := eHentaiLock.TryLock(ep.GIds)
	if len(failedGIds) > 0 {
		ctx.SendMsgReplyf("[EHentai] locks of galleries %v are holding by other users", failedGIds)
		return
	}
	defer eHentaiLock.Unlock(ep.GIds)

	respDownload, err := ctx.SendMsg("[EHentai] 下载中...")
	ctx.Std.DeleteMsg(respFetching.MessageId)
	if err != nil {
		logEHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	// 所有页一起发一个合并转发
	forward := message.SegmentArray{}
	for i, gId := range ep.GIds {
		forward.Append(message.Node3(uid, name, ep.GalleryHeader(i)))

		// 每最多 [EHentaiConfig.ForwardMsgBatchSize] 张图打包一个气泡
		bp := utils.NewBatchPacker(mangaConfig.ForwardMsgBatchSize, eHentaiPackFuncWraper(
			func(segChain message.SegmentArray) {
				forward.Append(message.Node3(uid, name, segChain))
			},
		))
		for page, err := range ep.DownloadIter(i) {
			if err != nil {
				ctx.SendMsgf("[EHentai] gallery %d failed to download page %d: %v", gId, page.PageNum, err)
				return
			}
			if pDownload {
				continue
			}
			bp.Append(page)
		}
		bp.Pack() // pack last
	}

	if pDownload {
		_, err := ctx.SendMsg("[EHentai] 下载完成")
		if err != nil {
			logEHentai.Error().
				Err(err).
				Msg("failed to send msg")
		}
		return
	}

	respSend, err := ctx.SendMsg("[EHentai] 发送中...")
	ctx.Std.DeleteMsg(respDownload.MessageId)
	if err != nil {
		logEHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	ts := time.Now()
	respSendForward, err := ctx.SendForwardMsgAuto(forward)
	ctx.Std.DeleteMsg(respSend.MessageId)
	if err != nil {
		logEHentai.Error().
			Err(err).
			Msg("failed to send forward msg")
		ctx.SendMsg("[EHentai] 发送失败")
		return
	}
	respRecallHint, _ := ctx.SendMsgReplyf("[EHentai] 请转发查收！一分钟后撤回\n%s", time.Since(ts))

	if !pNoRecall {
		registerTimerRecall(respSendForward.MessageId)
		if respRecallHint != nil {
			registerTimerRecall(respRecallHint.MessageId)
		}
	}
}

// 打包回调
func eHentaiPackFuncWraper(f func(segChain message.SegmentArray)) func([]eh.PageData) {
	return func(batch []eh.PageData) {
		// 从一段页码中格式化成字符串
		sb := strings.Builder{}
		sb.WriteString("P")
		start, end := batch[0].PageNum, batch[0].PageNum
		for i := 1; i < len(batch); i++ {
			p := batch[i].PageNum
			if p == end+1 {
				// 如果当前页码是连续的, 更新结束页码
				end = p
			} else {
				// 如果不连续, 写入当前范围
				writePageNum(&sb, start, end)
				sb.WriteString(", ")
				// 更新新的范围
				start, end = p, p
			}
		}
		// 写入最后一个范围
		writePageNum(&sb, start, end)

		segChain := make(message.SegmentArray, 0, 1+len(batch))
		segChain.Append(message.Text(sb.String()))
		for _, p := range batch {
			segChain.Append(message.Image(p.Data))
		}

		f(segChain)
	}
}

var eHentaiLock Locker[int]

type EHentaiParseOption struct {
	DlAll        bool // 绕过 [MangaConfig.MaxForwardImages] 限制
	ToJpg        bool
	Salt         bool // 加盐修改哈希
	ToPdf        bool // 转为 pdf, 此时需要jpg且不加盐
	Purge        bool // 无视缓存
	DownloadOnly bool

	IsGalleryParse  bool
	IsPageParse     bool
	SliceSyntaxUsed bool
}

type EHentaiParse struct {
	GIds        []int // 准备解析的画廊, 作为 map 的 key
	GalleryUrls map[int]string
	Galleries   map[int]*eh.GalleryDetails

	Options EHentaiParseOption

	FetchCosts    map[int]time.Duration
	DownloadCosts map[int]time.Duration
	// UploadCosts map[int]time.Duration

	Ctx context.Context
}

func NewEhGalleryParse(ctx context.Context, galleryUrls []string, sss []slicesyntax.SliceSyntaxes, options EHentaiParseOption) (ep *EHentaiParse, err error) {
	options.IsGalleryParse = true
	for i := range galleryUrls {
		galleryUrls[i] = "https://" + galleryUrls[i]
	}

	ep = &EHentaiParse{
		GIds:        make([]int, len(galleryUrls)),
		GalleryUrls: make(map[int]string, len(galleryUrls)),
		Galleries:   make(map[int]*eh.GalleryDetails, len(galleryUrls)),

		Options: options,

		FetchCosts:    make(map[int]time.Duration, len(galleryUrls)),
		DownloadCosts: make(map[int]time.Duration, len(galleryUrls)),
		// UploadCosts: make(map[int]time.Duration, len(galleryUrls)),

		Ctx: ctx,
	}

	// 按原顺序收集画廊 ID 和 URL
	// 画廊 ID 作为 map 的 key
	for i, gUrl := range galleryUrls {
		g := eh.UrlToGallery(gUrl)
		ep.GIds[i] = g.GalleryId
		ep.GalleryUrls[g.GalleryId] = gUrl
	}

	// 获取画廊数据 同时解析 [slicesyntax.SliceSyntaxes]
	for i, galleryUrl := range galleryUrls {
		tStart := time.Now()

		g, err := eh.FetchGalleryDetails(ep.Ctx, galleryUrl)
		if err != nil {
			return nil, err
		}

		if sss[i] != nil {
			options.SliceSyntaxUsed = true
			g.PageUrls = slicesyntax.DoIndexes(g.PageUrls, sss[i].ToIndexesNoRepeat(len(g.PageUrls)))
		}
		if !options.DlAll {
			if options.ToPdf {
				if len(g.PageUrls) > mangaConfig.MaxPdfImages {
					g.PageUrls = g.PageUrls[:mangaConfig.MaxPdfImages]
				}
			} else {
				if len(g.PageUrls) > mangaConfig.MaxForwardImages {
					g.PageUrls = g.PageUrls[:mangaConfig.MaxForwardImages]
				}
			}
		}
		ep.Galleries[g.GalleryId] = &g

		ep.FetchCosts[g.GalleryId] = time.Since(tStart)
	}

	return ep, nil
}

func NewEhPageParse(ctx context.Context, pageUrls []string, options EHentaiParseOption) (ep *EHentaiParse, err error) {
	options.IsPageParse = true

	// 从 pageUrls 中整理出画廊, 并保留域名信息
	gPageUrl := map[int]string{} // for token and domain
	gPages := map[int][]int{}
	for _, url := range pageUrls {
		g := eh.UrlToPage(url)

		if _, ok := gPageUrl[g.GalleryId]; !ok {
			gPageUrl[g.GalleryId] = url
		}

		pages := gPages[g.GalleryId]
		pages = append(pages, g.PageNum-1) // '1' indexed
		gPages[g.GalleryId] = pages
	}

	gIds := slices.Sorted(maps.Keys(gPages))

	ep = &EHentaiParse{
		GIds:        gIds,
		GalleryUrls: make(map[int]string, len(gIds)),
		Galleries:   make(map[int]*eh.GalleryDetails, len(gIds)),

		Options: options,

		FetchCosts:    make(map[int]time.Duration, len(gIds)),
		DownloadCosts: make(map[int]time.Duration, len(gIds)),
		// UploadCosts: make(map[int]time.Duration, len(gIds)),

		Ctx: ctx,
	}

	// 从每个画廊中取一个 P
	// 获取画廊 token
	pageList := make([]eh.PageList, len(ep.GIds))
	for i, gId := range ep.GIds {
		pageList[i] = eh.UrlToPage(gPageUrl[gId])
	}
	tokens, err := eh.PostGalleryToken(ep.Ctx, pageList...)
	if err != nil {
		return nil, err
	}
	if len(tokens) != len(ep.GIds) {
		return nil, fmt.Errorf("len(tokens) %d != len(ep.gIds) %d", len(tokens), len(ep.GIds))
	}

	// 整理画廊 url
	for _, token := range tokens {
		const urlLen = len("e-hentai.org")
		g := token.ToGallery()
		ep.GalleryUrls[g.GalleryId] = fmt.Sprintf("https://%s/g/%d/%s", gPageUrl[g.GalleryId][:urlLen], g.GalleryId, g.GalleryToken)
	}

	// 获取画廊元数据
	for _, gId := range ep.GIds {
		tStart := time.Now()

		g, err := eh.FetchGalleryDetails(ep.Ctx, ep.GalleryUrls[gId])
		if err != nil {
			return nil, err
		}

		pages := gPages[g.GalleryId]
		if max := slices.Max(pages); max >= len(g.PageUrls) {
			return nil, fmt.Errorf("max page [%d] out of bounds: %d", max, len(g.PageUrls))
		}
		if min := slices.Min(pages); min < 0 {
			return nil, fmt.Errorf("min page [%d] out of bounds: %d", min, len(g.PageUrls))
		}
		g.PageUrls = slicesyntax.DoIndexes(g.PageUrls, pages)
		ep.Galleries[g.GalleryId] = &g

		ep.FetchCosts[g.GalleryId] = time.Since(tStart)
	}

	return ep, nil
}

func (ep *EHentaiParse) DownloadingHint(i int) string {
	gId := ep.GIds[i]
	if len(ep.GIds) == 1 {
		return fmt.Sprintf("%d 获取耗时%s\n下载中...", gId, ep.FetchCosts[gId])
	}
	return fmt.Sprintf("%d 获取耗时%s\n下载中...(%d/%d)", gId, ep.FetchCosts[gId], i+1, len(ep.GIds))
}

func (ep *EHentaiParse) SendingHint(i int) string {
	gId := ep.GIds[i]
	if len(ep.GIds) == 1 {
		return fmt.Sprintf("%d 下载耗时%s\n发送中...", gId, ep.DownloadCosts[gId])
	}
	return fmt.Sprintf("%d 下载耗时%s\n发送中...(%d/%d)", gId, ep.DownloadCosts[gId], i+1, len(ep.GIds))
}

func (ep *EHentaiParse) GalleryHeader(i int) (header message.SegmentArray) {
	gId := ep.GIds[i]
	g := ep.Galleries[gId]
	gUrl := ep.GalleryUrls[gId]
	gUrl = strings.TrimPrefix(gUrl, "https://")
	gUrl = strings.TrimSuffix(gUrl, "/")

	tagSb := strings.Builder{}
	tagSets := eh.TranslateTags(g.Tags).Set()
	for i, set := range tagSets {
		tagSb.WriteString(set.Namespace)
		tagSb.WriteString("：")
		tagSb.WriteString(strings.Join(set.Tags, "，"))
		if i != len(tagSets)-1 {
			tagSb.WriteString("\n")
		}
	}

	if !ep.Options.ToPdf {
		header.Append(message.Text("一分钟后撤回，提前转发走\n"))
	}

	header.Append(message.Textf(
		`%s
%s
%s | %.2f⭐ | %dP
%s`,
		g.TitleJpn,
		tagSb.String(),
		g.Cat, g.Rating, g.Length,
		gUrl,
	))

	if !ep.Options.DlAll {
		limit := 0
		if ep.Options.ToPdf {
			if g.Length > mangaConfig.MaxPdfImages {
				limit = mangaConfig.MaxPdfImages
			}
		} else if g.Length > mangaConfig.MaxForwardImages {
			limit = mangaConfig.MaxForwardImages
		}
		if limit != 0 {
			header.Append(message.Textf("\n只发送最多%d页", limit))
			if ep.Options.IsGalleryParse && !ep.Options.SliceSyntaxUsed {
				start := limit
				end := min(g.Length, limit*2)
				header.Append(message.Textf(
					"，发送 \"%s [%d:%d]\" 获取下一批",
					gUrl, start, end,
				))
			}
		}
	}

	return
}

func (ep *EHentaiParse) DownloadIter(i int) iter.Seq2[eh.PageData, error] {
	return func(yield func(eh.PageData, error) bool) {
		gId := ep.GIds[i]
		g := ep.Galleries[gId]
		// 手动为画廊创建缓存
		if eh.GetCache(gId) == nil {
			_, err := eh.CreateCacheFromUrl(ep.Ctx, ep.GalleryUrls[gId])
			if err != nil {
				yield(eh.PageData{}, err)
				return
			}
		}

		// [TODO] 使用画廊对象 like NHentai-go
		for page, err := range eh.DownloadPagesIter(context.Background(), g.PageUrls...) {
			if err != nil {
				if yield(page, err) {
					continue
				}
				return
			}

			if ep.Options.ToJpg {
				var data []byte
				data, _, err = imgToJpg(page.Data)
				if err != nil {
					if yield(page, err) {
						continue
					}
					return
				}
				page.Data = data
			}

			if ep.Options.Salt {
				page.Data = salt(page.Data, mangaConfig.SaltLength)
			}

			if !yield(page, nil) {
				return
			}
		}
	}
}

func (ep *EHentaiParse) BuildForward(i int, uid int, name string) (n int, forward message.SegmentArray, err error) {
	gId := ep.GIds[i]
	var bp *utils.BatchPacker[eh.PageData]
	if !ep.Options.DownloadOnly {
		bp = utils.NewBatchPacker(
			mangaConfig.ForwardMsgBatchSize,
			eHentaiPackFuncWraper(func(segChain message.SegmentArray) {
				forward.Append(message.Node3(uid, name, segChain))
			}),
		)
	}

	tStart := time.Now()
	for page, err := range ep.DownloadIter(i) {
		if err != nil {
			return n, nil, fmt.Errorf("gallery %d failed to download page %d: %w", gId, page.PageNum, err)
		}
		n++
		if !ep.Options.DownloadOnly {
			bp.Append(page)
		}
	}
	ep.DownloadCosts[gId] = time.Since(tStart)

	if !ep.Options.DownloadOnly {
		bp.Pack()
	}

	return n, forward, nil
}

func (ep *EHentaiParse) BuildPdf(i int, filename, filepath string) (n int, err error) {
	if ep.Options.Purge {
		_ = os.Remove(filepath)
	}
	_, err = os.Stat(filepath)
	if err != nil {
		gId := ep.GIds[i]
		domain, _, _ := eh.UrlGetGIdGToken(ep.GalleryUrls[gId])

		pdf := NewImagePdf()
		defer pdf.Close()

		tStart := time.Now()
		for page, err := range ep.DownloadIter(i) {
			if err != nil {
				return n, fmt.Errorf("gallery %d failed to download page %d: %w", gId, page.PageNum, err)
			}

			jpg, bounds, err := imgToJpg(page.Data)
			if err != nil {
				return n, fmt.Errorf("gallery %d failed to convert page %d to jpg: %w", gId, page.PageNum, err)
			}
			pdf.AddImage(jpg, bounds, domain+page.Page.String())

			n++
		}
		ep.DownloadCosts[gId] = time.Since(tStart)

		err = pdf.OutputFileAndClose(filepath)
		if err != nil {
			return n, fmt.Errorf("gallery %d failed to create pdf: %w", gId, err)
		}

		// 调用 qpdf 将 pdf 线性化
		out, err := exec.Command("qpdf", filepath, "--linearize", "--replace-input").CombinedOutput()
		if err != nil {
			logEHentai.Warn().
				Err(err).
				Int("gId", gId).
				Str("output", string(out)).
				Msg("failed to linearize pdf")
		}
	}
	return
}
