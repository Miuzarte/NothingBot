package main

import (
	"context"
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

	nh "github.com/Miuzarte/NHentai-go"
	nhApi "github.com/Miuzarte/NHentai-go/api"
	nhDl "github.com/Miuzarte/NHentai-go/downloader"
)

// 本文件的日志 scope
var logNHentai = logger.New("NHentai")

const (
	// [1]: site // unused
	// [2]: keyword
	NHENTAI_SEARCH_REGEXP = `(?i)(?:(N)(?:H|HENTAI)?)搜索?[\s:：]*(.+)`

	// https://nhentai.net/g/540994/
	// [1]: url self
	// [2]: slice syntax
	NHENTAI_GALLERY_URL_REGEXP = `(?s)(nhentai\.(?:net|xxx)/g/[0-9]+/?)` +
		`\s*(\[(?:-?\d*:?)*\](?:\[(?:-?\d*:?)*\])*)?`

	// https://nhentai.net/g/540994/1/
	// [0]: url self
	// [1]: gallery id // unused
	// [2]: page num // unused
	NHENTAI_PAGE_URL_REGEXP = `(?s)nhentai\.(?:net|xxx)/g/([0-9]+)/([0-9]+)/?`
)

var (
	nHentaiSearchReg     = regexp.MustCompile(NHENTAI_SEARCH_REGEXP)
	nHentaiGalleryUrlReg = regexp.MustCompile(NHENTAI_GALLERY_URL_REGEXP)
	nHentaiPageUrlReg    = regexp.MustCompile(NHENTAI_PAGE_URL_REGEXP)
)

const nHentaiMId ModuleId = "NHentai"

var (
	moduleNHentaiSearch = ModuleMeta{
		Name:       nHentaiMId.WithSuffix("Search"),
		Desc:       "NHentai搜索",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg:    NHENTAI_SEARCH_REGEXP,
	}
	moduleNHentaiGalleryParse = ModuleMeta{
		Name:       nHentaiMId.WithSuffix("GalleryParse"),
		Desc:       "NHentai画廊链接解析",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg: NHENTAI_GALLERY_URL_REGEXP +
			"\n\n参数：" +
			"\n链接尾随(一或多个) [n:m] / [n:] / [:m] 以指定下载的页码范围, 支持负索引" +
			"\n--pdf 合并为 Pdf 文件发送" +
			"\n--noupload 不上传 Pdf 至文件",
	}
	moduleNHentaiPageParse = ModuleMeta{
		Name:       nHentaiMId.WithSuffix("PageParse"),
		Desc:       "NHentai画廊页链接解析",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg:    NHENTAI_PAGE_URL_REGEXP,
	}
)

var moduleNHentai = Module{
	ModuleMeta: ModuleMeta{
		Name:   nHentaiMId,
		Hidden: true,
	},
	Priority: 1, // after [moduleManga]
	Disable:  env.Testing,
	SubModules: []*ModuleMeta{
		&moduleNHentaiSearch,
		&moduleNHentaiGalleryParse,
		&moduleNHentaiPageParse,
	},
}

func init() {
	NoBuildPrintFile("M_NHentai.go")

	moduleNHentai.Init = initNHentai
	moduleNHentai.ReInit = initNHentai
	modules.Add(&moduleNHentai)
}

func initNHentai() {
	if mangaConfig.Threads > 0 {
		nh.SetThreads(mangaConfig.Threads)
	}
	nh.SetUseEnvProxy(mangaConfig.UseEnvProxy)
	nh.SetApiKey(mangaConfig.NHentaiApiKey)

	onebot.AddMatcher(moduleNHentaiSearch.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return mangaConfig.NHentaiEnabled && mangaConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(nHentaiSearchReg).
		Do(moduleNHentai.RWMuWrap(ctxNHentaiSearch)),
	)
	onebot.AddMatcher(moduleNHentaiGalleryParse.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return mangaConfig.NHentaiEnabled && mangaConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(nHentaiGalleryUrlReg).
		Do(moduleNHentai.RWMuWrap(ctxNHentaiGalleryParse)),
	)
	onebot.AddMatcher(moduleNHentaiPageParse.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return mangaConfig.NHentaiEnabled && mangaConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(nHentaiPageUrlReg).
		Do(moduleNHentai.RWMuWrap(ctxNHentaiPageParse)),
	)
}

func ctxNHentaiSearch(ctx *EasyOnebot.Ctx) {
	submatch := ctx.Submatches.Get(moduleNHentaiSearch.Name.String())[0]
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))

	keyword := submatch[2]

	respSearch, err := ctx.SendMsg("[NHentai] 搜索中...")
	if err != nil {
		logNHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	ms := NewMangaSearch(ctx, tctx, MANGA_SITE_NH, keyword)
	forward, err := ms.Do()
	if err != nil {
		ctx.SendMsgf("[NHentai] 搜索失败：%v", err)
		return
	}

	ts := time.Now()
	respSendForward, err := ctx.SendForwardMsgAuto(forward)
	ctx.Std.DeleteMsg(respSearch.MessageId)
	if err != nil {
		ctx.SendMsg("[NHentai] 搜索结果发送失败")
		return
	}
	respRecallHint, _ := ctx.SendMsgReplyf("[NHentai] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

	if !pNoRecall {
		registerTimerRecall(respSendForward.MessageId)
		if respRecallHint != nil {
			registerTimerRecall(respRecallHint.MessageId)
		}
	}
}

func ctxNHentaiGalleryParse(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatches := ctx.Submatches.Get(moduleNHentaiGalleryParse.Name.String())
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
		ctx.SendMsgf("[NHentai] ☝️哒咩！%d个画廊太多了", len(galleryUrls))
		return
	}

	respFetch, err := ctx.SendMsg("[NHentai] 获取中...")
	if err != nil {
		logNHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	npOp := NHentaiParseOption{
		DlAll:        pAll,
		ToPdf:        pUsePdf,
		Purge:        pPurge,
		DownloadOnly: pDownload,
	}
	if npOp.ToPdf {
		npOp.ToJpg = true
		npOp.Salt = false
	} else {
		npOp.ToJpg = false
		npOp.Salt = true
	}
	np, err := NewNhGalleryParse(tctx, galleryUrls, sss, npOp)
	if err != nil {
		ctx.SendMsgReplyf("[NHentai] 解析失败：%v", err)
		return
	}

	failedGIds := nHentaiLock.TryLock(np.GIds)
	if len(failedGIds) > 0 {
		ctx.SendMsgReplyf("[NHentai] locks of gallery(s) %v are holding by other users", failedGIds)
		return
	}
	defer nHentaiLock.Unlock(np.GIds)

	// 每个画廊发一个合并转发
	for i, gId := range np.GIds {
		respDownload, err := ctx.SendMsg(np.DownloadingHint(i))
		if i == 0 {
			ctx.Std.DeleteMsg(respFetch.MessageId)
		}
		if err != nil {
			logNHentai.Error().
				Err(err).
				Msg("failed to send msg")
			return
		}

		// 使用 pdf 时也用合并转发发送画廊信息
		forward := message.SegmentArray{
			message.Node3(uid, name, np.GalleryHeader(i)),
		}

		if !pUsePdf {
			n, nodes, err := np.BuildForward(i, uid, name)
			if err != nil {
				ctx.SendMsgf("[NHentai] %v", err)
				return
			}

			if pDownload {
				_, err := ctx.SendMsgReplyf("[NHentai] 画廊 %d 下载完成 (%d)", gId, n)
				if err != nil {
					logNHentai.Error().
						Err(err).
						Msg("failed to send msg")
				}
				continue
			}

			respSend, err := ctx.SendMsg(np.SendingHint(i))
			ctx.Std.DeleteMsg(respDownload.MessageId)
			if err != nil {
				logNHentai.Error().
					Err(err).
					Msg("failed to send msg")
				return
			}

			ts := time.Now()
			respSendForward, err := ctx.SendForwardMsgAuto(append(forward, nodes...))
			ctx.Std.DeleteMsg(respSend.MessageId)
			if err != nil {
				ctx.SendMsgf("[NHentai] 画廊 %d 发送失败", gId)
				return
			}
			respRecallHint, _ := ctx.SendMsgReplyf("[NHentai] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

			if !pNoRecall {
				registerTimerRecall(respSendForward.MessageId)
				if respRecallHint != nil {
					registerTimerRecall(respRecallHint.MessageId)
				}
			}

		} else {
			filename := fmt.Sprintf("%d.pdf", gId)
			filepath := filepath.Join(env.WorkDir, mangaConfig.NHentaiCacheDir, filename)
			_, err := np.BuildPdf(i, filename, filepath)
			if err != nil {
				ctx.SendMsgf("[NHentai] %v", err)
				return
			}

			// 发送画廊信息
			_, err = ctx.SendForwardMsgAuto(forward)
			if err != nil {
				logNHentai.Error().
					Err(err).
					Int("gId", gId).
					Msg("failed to send msg")
				ctx.SendMsgf("[NHentai] 画廊 %d 信息合并转发发送失败", gId)
			}

			// 发送直链
			_, err = ctx.SendMsgf("[NHentai] 直接查看: https://nhentai.miuzarte.top/%s", filename)
			if err != nil {
				logNHentai.Error().
					Err(err).
					Int("gId", gId).
					Msg("failed to send msg")
				ctx.SendMsgf("[NHentai] 画廊 %d 直链发送失败", gId)
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
					logNHentai.Warn().
						Str("type", ctx.Event.MessageType).
						Msg("unsupported message type")
					ctx.SendMsgf("[NHentai] 不支持的消息类型：%s", ctx.Event.MessageType)
					return
				}
				if err != nil {
					logNHentai.Error().
						Err(err).
						Int("gId", gId).
						Msg("failed to upload pdf")
					ctx.SendMsgf("[NHentai] 画廊 %d pdf上传失败：%v", gId, err)
					return
				}
				ctx.SendMsgReplyf("[NHentai] 画廊 %d pdf上传完毕！\n(%s)", gId, time.Since(ts))
			}

		}

	}
}

func ctxNHentaiPageParse(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatches := ctx.Submatches.Get(moduleNHentaiPageParse.Name.String())
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pUsePdf := strings.Contains(lowerRm, "--pdf")
	pDownload := strings.Contains(lowerRm, "--download") // 仅下载
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))

	if pUsePdf {
		ctx.SendMsgReply("[NHentai] pdf is not supported for pages link")
		return
	}

	if len(submatches) > mangaConfig.MaxForwardImages && !ctx.IsSuperuser {
		ctx.SendMsgf("[NHentai] ☝️哒咩！%d页太多了", len(submatches))
		return
	}

	pageUrlSet := utils.Set[string]{}
	for _, submatch := range submatches {
		pageUrlSet.Add(submatch[0])
	}
	pageUrls := pageUrlSet.Get()
	slices.Sort(pageUrls)

	respFetching, err := ctx.SendMsg("[NHentai] 获取中...")
	if err != nil {
		logNHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	npOp := NHentaiParseOption{
		Salt:         true,
		ToJpg:        false,
		DownloadOnly: pDownload,
	}
	np, err := NewNhPageParse(tctx, pageUrls, npOp)
	if err != nil {
		ctx.SendMsgReplyf("[NHentai] 解析失败：%v", err)
		return
	}

	failedGIds := nHentaiLock.TryLock(np.GIds)
	if len(failedGIds) > 0 {
		ctx.SendMsgReplyf("[NHentai] locks of galleries %v are holding by other users", failedGIds)
		return
	}
	defer nHentaiLock.Unlock(np.GIds)

	respDownload, err := ctx.SendMsg("[NHentai] 下载中...")
	ctx.Std.DeleteMsg(respFetching.MessageId)
	if err != nil {
		logNHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	// 所有页一起发一个合并转发
	forward := message.SegmentArray{}
	for i, gId := range np.GIds {
		forward.Append(message.Node3(uid, name, np.GalleryHeader(i)))

		// 每最多 [EHentaiConfig.ForwardMsgBatchSize] 张图打包一个气泡
		bp := utils.NewBatchPacker(mangaConfig.ForwardMsgBatchSize, nHentaiPackFuncWraper(
			func(segChain message.SegmentArray) {
				forward.Append(message.Node3(uid, name, segChain))
			},
		))
		for img, err := range np.DownloadIter(i) {
			if err != nil {
				ctx.SendMsgf("[NHentai] gallery %d failed to download image %s: %v", gId, img.Name, err)
				return
			}
			if pDownload {
				continue
			}
			bp.Append(img)
		}
		bp.Pack() // pack last
	}

	if pDownload {
		_, err := ctx.SendMsg("[NHentai] 下载完成")
		if err != nil {
			logNHentai.Error().
				Err(err).
				Msg("failed to send msg")
		}
		return
	}

	respSend, err := ctx.SendMsg("[NHentai] 发送中...")
	ctx.Std.DeleteMsg(respDownload.MessageId)
	if err != nil {
		logNHentai.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	ts := time.Now()
	respSendForward, err := ctx.SendForwardMsgAuto(forward)
	ctx.Std.DeleteMsg(respSend.MessageId)
	if err != nil {
		logNHentai.Error().
			Err(err).
			Msg("failed to send forward msg")
		ctx.SendMsg("[NHentai] 发送失败")
		return
	}
	respRecallHint, _ := ctx.SendMsgReplyf("[NHentai] 请转发查收！一分钟后撤回\n%s", time.Since(ts))

	if !pNoRecall {
		registerTimerRecall(respSendForward.MessageId)
		if respRecallHint != nil {
			registerTimerRecall(respRecallHint.MessageId)
		}
	}
}

// 打包回调
func nHentaiPackFuncWraper(f func(segChain message.SegmentArray)) func([]nhDl.Image) {
	return func(batch []nhDl.Image) {
		// 从一段页码中格式化成字符串
		sb := strings.Builder{}
		sb.WriteString("P")
		start, end := batch[0].P, batch[0].P
		for i := 1; i < len(batch); i++ {
			p := batch[i].P
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

var nHentaiLock Locker[int]

type NHentaiParseOption struct {
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

type NHentaiParse struct {
	GIds        []int // 准备解析的画廊, 作为 map 的 key
	GalleryUrls map[int]string
	Galleries   map[int]*nhApi.GalleryDetailResponse

	Options NHentaiParseOption

	FetchCosts    map[int]time.Duration
	DownloadCosts map[int]time.Duration
	// UploadCosts map[int]time.Duration

	Ctx context.Context
}

func NewNhGalleryParse(ctx context.Context, galleryUrls []string, sss []slicesyntax.SliceSyntaxes, options NHentaiParseOption) (np *NHentaiParse, err error) {
	options.IsGalleryParse = true
	for i := range galleryUrls {
		galleryUrls[i] = "https://" + galleryUrls[i]
	}

	np = &NHentaiParse{
		GIds:        make([]int, len(galleryUrls)),
		GalleryUrls: make(map[int]string, len(galleryUrls)),
		Galleries:   make(map[int]*nhApi.GalleryDetailResponse, len(galleryUrls)),

		Options: options,

		FetchCosts:    make(map[int]time.Duration, len(galleryUrls)),
		DownloadCosts: make(map[int]time.Duration, len(galleryUrls)),
		// UploadCosts: make(map[int]time.Duration, len(galleryUrls)),

		Ctx: ctx,
	}

	// 按原顺序收集画廊 ID 和 URL
	// 画廊 ID 作为 map 的 key
	for i, gUrl := range galleryUrls {
		gId, _, err := nHUrlDeconstruct(gUrl)
		if err != nil {
			return nil, err
		}
		np.GIds[i] = gId
		np.GalleryUrls[gId] = gUrl
	}

	// 获取画廊数据 同时解析 [slicesyntax.SliceSyntaxes]
	for i, gId := range np.GIds {
		tStart := time.Now()

		g, err := nh.GetGallery(np.Ctx, gId)
		if err != nil {
			return nil, err
		}

		if g.Pages != nil {
			pages := *g.Pages
			if sss[i] != nil {
				options.SliceSyntaxUsed = true
				pages = slicesyntax.DoIndexes(pages, sss[i].ToIndexesNoRepeat(len(pages)))
			}
			if !options.DlAll {
				if options.ToPdf {
					if len(pages) > mangaConfig.MaxPdfImages {
						pages = pages[:mangaConfig.MaxPdfImages]
					}
				} else {
					if len(pages) > mangaConfig.MaxForwardImages {
						pages = pages[:mangaConfig.MaxForwardImages]
					}
				}
			}
			*g.Pages = pages
		}
		np.Galleries[g.Id] = g

		np.FetchCosts[gId] = time.Since(tStart)
	}

	return np, nil
}

func NewNhPageParse(ctx context.Context, pageUrls []string, options NHentaiParseOption) (np *NHentaiParse, err error) {
	options.IsPageParse = true

	// 从 pageUrls 中整理出画廊
	gPages := map[int][]int{}
	for _, url := range pageUrls {
		gId, pNum, err := nHUrlDeconstruct(url)
		if err != nil {
			return nil, err
		}

		pages := gPages[gId]
		pages = append(pages, pNum-1) // '1' indexed
		gPages[gId] = pages
	}

	gIds := slices.Sorted(maps.Keys(gPages))

	np = &NHentaiParse{
		GIds:        gIds,
		GalleryUrls: make(map[int]string, len(gIds)),
		Galleries:   make(map[int]*nhApi.GalleryDetailResponse, len(gIds)),

		Options: options,

		FetchCosts:    make(map[int]time.Duration, len(gIds)),
		DownloadCosts: make(map[int]time.Duration, len(gIds)),
		// UploadCosts: make(map[int]time.Duration, len(gIds)),

		Ctx: ctx,
	}

	// 获取画廊元数据
	for _, gId := range np.GIds {
		np.GalleryUrls[gId] = fmt.Sprintf("https://nhentai.net/g/%d/", gId)

		tStart := time.Now()

		g, err := nh.GetGallery(np.Ctx, gId)
		if err != nil {
			return nil, err
		}
		if g.Pages == nil {
			return nil, fmt.Errorf("gallery %d has no pages", g.Id)
		}
		gPagesSlice := *g.Pages

		pages := gPages[g.Id]
		if max := slices.Max(pages); max >= len(gPagesSlice) {
			return nil, fmt.Errorf("max page [%d] out of bounds: %d", max, len(gPagesSlice))
		}
		if min := slices.Min(pages); min < 0 {
			return nil, fmt.Errorf("min page [%d] out of bounds: %d", min, len(gPagesSlice))
		}
		*g.Pages = slicesyntax.DoIndexes(gPagesSlice, pages)
		np.Galleries[g.Id] = g

		np.FetchCosts[gId] = time.Since(tStart)
	}

	return np, nil
}

func (np *NHentaiParse) DownloadingHint(i int) string {
	gId := np.GIds[i]
	if len(np.GIds) == 1 {
		return fmt.Sprintf("%d 获取耗时%s\n下载中...", gId, np.FetchCosts[gId])
	}
	return fmt.Sprintf("%d 获取耗时%s\n下载中...(%d/%d)", gId, np.FetchCosts[gId], i+1, len(np.GIds))
}

func (np *NHentaiParse) SendingHint(i int) string {
	gId := np.GIds[i]
	if len(np.GIds) == 1 {
		return fmt.Sprintf("%d 下载耗时%s\n发送中...", gId, np.DownloadCosts[gId])
	}
	return fmt.Sprintf("%d 下载耗时%s\n发送中...(%d/%d)", gId, np.DownloadCosts[gId], i+1, len(np.GIds))
}

func (np *NHentaiParse) GalleryHeader(i int) (header message.SegmentArray) {
	gId := np.GIds[i]
	g := np.Galleries[gId]
	gUrl := np.GalleryUrls[gId]
	gUrl = strings.TrimPrefix(gUrl, "https://")
	gUrl = strings.TrimSuffix(gUrl, "/")

	if !np.Options.ToPdf {
		header.Append(message.Text("一分钟后撤回，提前转发走\n"))
	}

	japaneseTitle := ""
	if g.Title.Japanese != nil {
		japaneseTitle = *g.Title.Japanese
	}
	header.Append(message.Textf(
		`%s
%s
%d❤️ | %dP
%s`,
		japaneseTitle,
		translateNhTags(g.Tags),
		g.NumFavorites, g.NumPages,
		gUrl,
	))

	if !np.Options.DlAll {
		limit := 0
		if np.Options.ToPdf {
			if g.NumPages > mangaConfig.MaxPdfImages {
				limit = mangaConfig.MaxPdfImages
			}
		} else if g.NumPages > mangaConfig.MaxForwardImages {
			limit = mangaConfig.MaxForwardImages
		}
		if limit != 0 {
			header.Append(message.Textf("\n只发送最多%d页", limit))
			if np.Options.IsGalleryParse && !np.Options.SliceSyntaxUsed {
				start := limit
				end := min(g.NumPages, limit*2)
				header.Append(message.Textf(
					"，发送 \"%s [%d:%d]\" 获取下一批",
					gUrl, start, end,
				))
			}
		}
	}

	return
}

func (np *NHentaiParse) DownloadIter(i int) iter.Seq2[nhDl.Image, error] {
	return func(yield func(nhDl.Image, error) bool) {
		gId := np.GIds[i]
		g := np.Galleries[gId]
		for img, err := range g.DownloadPagesIter(np.Ctx) {
			if err != nil {
				if yield(img, err) {
					continue
				}
				return
			}

			if np.Options.ToJpg {
				var data []byte
				data, _, err = imgToJpg(img.Data)
				if err != nil {
					if yield(img, err) {
						continue
					}
					return
				}
				img.Data = data
			}

			if np.Options.Salt {
				img.Data = salt(img.Data, mangaConfig.SaltLength)
			}

			if !yield(img, nil) {
				return
			}
		}
	}
}

func (np *NHentaiParse) BuildForward(i int, uid int, name string) (n int, forward message.SegmentArray, err error) {
	gId := np.GIds[i]
	var bp *utils.BatchPacker[nhDl.Image]
	if !np.Options.DownloadOnly {
		bp = utils.NewBatchPacker(mangaConfig.ForwardMsgBatchSize,
			nHentaiPackFuncWraper(func(segChain message.SegmentArray) {
				forward.Append(message.Node3(uid, name, segChain))
			}),
		)
	}

	tStart := time.Now()
	for img, err := range np.DownloadIter(i) {
		if err != nil {
			return n, nil, fmt.Errorf("gallery %d failed to download image %s: %w", gId, img.Name, err)
		}

		n++

		if !np.Options.DownloadOnly {
			bp.Append(img)
		}
	}
	np.DownloadCosts[gId] = time.Since(tStart)

	if !np.Options.DownloadOnly {
		bp.Pack()
	}

	return n, forward, nil
}

func (np *NHentaiParse) BuildPdf(i int, filename, filepath string) (n int, err error) {
	if np.Options.Purge {
		_ = os.Remove(filepath)
	}
	_, err = os.Stat(filepath)
	if err != nil {
		gId := np.GIds[i]

		pdf := NewImagePdf()
		defer pdf.Close()

		tStart := time.Now()
		for img, err := range np.DownloadIter(i) {
			if err != nil {
				return n, fmt.Errorf("gallery %d failed to download image %s: %w", gId, img.Name, err)
			}

			jpg, bounds, err := imgToJpg(img.Data)
			if err != nil {
				return n, fmt.Errorf("gallery %d failed to convert image %s to jpg: %w", gId, img.Name, err)
			}
			pdf.AddImage(jpg, bounds, fmt.Sprintf("https://nhentai.net/g/%d/%d", gId, n+1))

			n++
		}
		np.DownloadCosts[gId] = time.Since(tStart)

		err = pdf.OutputFileAndClose(filepath)
		if err != nil {
			return n, fmt.Errorf("gallery %d failed to create pdf: %w", gId, err)
		}

		// 调用 qpdf 将 pdf 线性化
		out, err := exec.Command("qpdf", filepath, "--linearize", "--replace-input").CombinedOutput()
		if err != nil {
			logNHentai.Warn().
				Err(err).
				Int("gId", gId).
				Str("output", string(out)).
				Msg("failed to linearize pdf")
		}
	}
	return
}
