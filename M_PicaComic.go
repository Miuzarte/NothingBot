package main

import (
	"context"
	"fmt"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"
	"NothingBot_v4/slicesyntax"
	"NothingBot_v4/utils"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	pc "github.com/Miuzarte/PicaComic-go"
)

// 本文件的日志 scope
var logPicaComic = logger.New("PicaComic")

const (
	// [1]: site // unused
	// [2]: keyword
	PICACOMIC_SEARCH_REGEXP = `(?i)(BK|PC|PICA|哔咔)搜索?[\s:：]*(.+)`

	// `630f6170c0b3ab7d08f3da8a` (hex)
	// [1]: pcId
	// [2]: epId
	// [3]: slice syntax
	PICACOMIC_PARSE_REGEXP = `pica://([0-9a-f]{24})(?:/(\d+))?` +
		`\s*(\[(?:-?\d*:?)*\](?:\[(?:-?\d*:?)*\])*)?`
)

var (
	picaComicSearchReg = regexp.MustCompile(PICACOMIC_SEARCH_REGEXP)
	picaComicParseReg  = regexp.MustCompile(PICACOMIC_PARSE_REGEXP)
)

const picaComicMId ModuleId = "PicaComic"

var (
	modulePicaComicSearch = ModuleMeta{
		Name:       picaComicMId.WithSuffix("Search"),
		Desc:       "哔咔漫画搜索",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg:    PICACOMIC_SEARCH_REGEXP,
	}
	modulePicaComicParse = ModuleMeta{
		Name:       picaComicMId.WithSuffix("Parse"),
		Desc:       "哔咔漫画解析",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg: PICACOMIC_PARSE_REGEXP +
			"\n\n参数：" +
			"\n--pdf 合并为 Pdf 文件发送" +
			"\n--noupload 不上传 Pdf 至文件",
	}
)

var modulePicaComic = Module{
	ModuleMeta: ModuleMeta{
		Name:   picaComicMId,
		Hidden: true,
	},
	Priority: 1, // after [moduleManga]
	Disable:  env.Testing,
	SubModules: []*ModuleMeta{
		&modulePicaComicSearch,
		&modulePicaComicParse,
	},
}

func init() {
	NoBuildPrintFile("M_PicaComic.go")

	modulePicaComic.Init = initPicaComic
	modulePicaComic.ReInit = initPicaComic
	modules.Add(&modulePicaComic)
}

func initPicaComic() {
	if mangaConfig.Threads > 0 {
		pc.SetThreads(mangaConfig.Threads)
	}
	pc.SetUseEnvProxy(mangaConfig.UseEnvProxy)
	if mangaConfig.PicaComicCookie.Token != "" {
		pc.SetToken(mangaConfig.PicaComicCookie.Token)
	} else if mangaConfig.PicaComicCookie.Account != "" && mangaConfig.PicaComicCookie.Password != "" {
		resp, err := pc.SignIn(context.Background(), mangaConfig.PicaComicCookie.Account, mangaConfig.PicaComicCookie.Password)
		if err != nil {
			logPicaComic.Warn().
				Err(err).
				Msg("failed to sign in")
			return
		} else {
			logPicaComic.Info().
				Str("token", resp.Token).
				Msg("account token")
		}
	} else {
		logPicaComic.Warn().Msg("not signed in")
		return
	}

	onebot.AddMatcher(modulePicaComicSearch.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return mangaConfig.PicaComicEnabled && mangaConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(picaComicSearchReg).
		Do(modulePicaComic.RWMuWrap(ctxPicaComicSearch)),
	)
	onebot.AddMatcher(modulePicaComicParse.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return mangaConfig.PicaComicEnabled && mangaConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(picaComicParseReg).
		Do(modulePicaComic.RWMuWrap(ctxPicaComicParse)),
	)
}

func ctxPicaComicSearch(ctx *EasyOnebot.Ctx) {
	submatch := ctx.Submatches.Get(modulePicaComicSearch.Name.String())[0]
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))

	keyword := submatch[2]

	respSearch, err := ctx.SendMsg("[PicaComic] 搜索中...")
	if err != nil {
		logPicaComic.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	ms := NewMangaSearch(ctx, tctx, MANGA_SITE_PC, keyword)
	forward, err := ms.Do()
	if err != nil {
		ctx.SendMsgf("[PicaComic] 搜索失败：%v", err)
		return
	}

	ts := time.Now()
	respSendForward, err := ctx.SendForwardMsgAuto(forward)
	ctx.Std.DeleteMsg(respSearch.MessageId)
	if err != nil {
		ctx.SendMsg("[PicaComic] 搜索结果发送失败")
		return
	}
	respRecallHint, _ := ctx.SendMsgReplyf("[PicaComic] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

	if !pNoRecall {
		registerTimerRecall(respSendForward.MessageId)
		if respRecallHint != nil {
			registerTimerRecall(respRecallHint.MessageId)
		}
	}
}

type PicaComicSubmatch struct {
	PcId string
	EpId int
}

func ctxPicaComicParse(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatches := ctx.Submatches.Get(modulePicaComicParse.Name.String())
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pUsePdf := strings.Contains(lowerRm, "--pdf")
	pNoUpload := strings.Contains(lowerRm, "--noupload") || strings.Contains(lowerRm, "--no-upload")
	pPurge := strings.Contains(lowerRm, "--purge")
	pDownload := strings.Contains(lowerRm, "--download") // 仅下载
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))
	pAll := strings.Contains(lowerRm, "--all") && ctx.IsSuperuser

	pcs := make([]PicaComicSubmatch, 0, len(submatches))
	sss := make([]slicesyntax.SliceSyntaxes, len(submatches))
	for i, submatch := range submatches {
		epId := 1
		if submatch[2] != "" {
			var err error
			epId, err = strconv.Atoi(submatch[2])
			if err != nil {
				ctx.SendMsgf("[PicaComic] [FIXME] unexpected non-numerical epId: %s", submatch[2])
				return
			}
		}
		pcs = append(pcs, PicaComicSubmatch{
			submatch[1],
			epId,
		})
		if submatch[3] != "" {
			sss[i] = slicesyntax.ParseMulti(submatch[2])
		}
	}

	if len(pcs) > 2 && !ctx.IsSuperuser {
		ctx.SendMsgf("[PicaComic] ☝️哒咩！%d个漫画太多了", len(pcs))
		return
	}

	respFetch, err := ctx.SendMsg("[PicaComic] 获取中...")
	if err != nil {
		logPicaComic.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	ppOp := PicaComicParseOption{
		DlAll:        pAll,
		ToPdf:        pUsePdf,
		Purge:        pPurge,
		DownloadOnly: pDownload,
	}
	if ppOp.ToPdf {
		ppOp.ToJpg = true
		ppOp.Salt = false
	} else {
		ppOp.ToJpg = false
		ppOp.Salt = true
	}
	pp, err := NewPicaComicParse(tctx, pcs, sss, ppOp)
	if err != nil {
		ctx.SendMsgReplyf("[PicaComic] 解析失败：%v", err)
		return
	}

	failedPcIds := picaComicLock.TryLock(pp.Pcs)
	if len(failedPcIds) > 0 {
		ctx.SendMsgReplyf("[PicaComic] locks of comic(s) %v are holding by other users", failedPcIds)
		return
	}
	defer picaComicLock.Unlock(pp.Pcs)

	// 每个漫画发一个合并转发
	for i, pcs := range pp.Pcs {
		respDownload, err := ctx.SendMsg(pp.DownloadingHint(i))
		if i == 0 {
			ctx.Std.DeleteMsg(respFetch.MessageId)
		}
		if err != nil {
			logPicaComic.Error().
				Err(err).
				Msg("failed to send msg")
			return
		}

		header, chapters := pp.ComicHeader(i)
		// 使用 pdf 时也用合并转发发送漫画信息
		forward := message.SegmentArray{
			message.Node3(uid, name, header),
		}
		if chapters != nil {
			forward.Append(
				message.Node3(uid, name, chapters),
			)
		}

		if !pUsePdf {
			n, nodes, err := pp.BuildForward(i, uid, name)
			if err != nil {
				ctx.SendMsgf("[PicaComic] %v", err)
				return
			}

			if pDownload {
				_, err := ctx.SendMsgReplyf("[PicaComic] 漫画 %s/%d 下载完成 (%d)", pcs.PcId, pcs.EpId, n)
				if err != nil {
					logPicaComic.Error().
						Err(err).
						Msg("failed to send msg")
				}
				continue
			}

			respSend, err := ctx.SendMsg(pp.SendingHint(i))
			ctx.Std.DeleteMsg(respDownload.MessageId)
			if err != nil {
				logPicaComic.Error().
					Err(err).
					Msg("failed to send msg")
				return
			}

			ts := time.Now()
			respSendForward, err := ctx.SendForwardMsgAuto(append(forward, nodes...))
			ctx.Std.DeleteMsg(respSend.MessageId)
			if err != nil {
				ctx.SendMsgf("[PicaComic] 漫画 %s/%d 发送失败", pcs.PcId, pcs.EpId)
				return
			}
			respRecallHint, _ := ctx.SendMsgReplyf("[PicaComic] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

			if !pNoRecall {
				registerTimerRecall(respSendForward.MessageId)
				if respRecallHint != nil {
					registerTimerRecall(respRecallHint.MessageId)
				}
			}

		} else {
			filename := fmt.Sprintf("%s_%d.pdf", pcs.PcId, pcs.EpId)
			filepath := filepath.Join(env.WorkDir, mangaConfig.PicaComicCacheDir, filename)
			_, err := pp.BuildPdf(i, filename, filepath)
			if err != nil {
				ctx.SendMsgf("[PicaComic] %v", err)
				return
			}

			// 发送漫画信息
			_, err = ctx.SendForwardMsgAuto(forward)
			if err != nil {
				logPicaComic.Error().
					Err(err).
					Str("pcId", pcs.PcId).
					Int("epId", pcs.EpId).
					Msg("failed to send msg")
				ctx.SendMsgf("[PicaComic] 漫画 %s/%d 信息合并转发发送失败", pcs.PcId, pcs.EpId)
			}

			// 发送直链
			_, err = ctx.SendMsgf("[PicaComic] 直接查看：https://picacomic.miuzarte.top/%s", filename)
			if err != nil {
				logPicaComic.Error().
					Err(err).
					Str("pcId", pcs.PcId).
					Int("epId", pcs.EpId).
					Msg("failed to send msg")
				ctx.SendMsgf("[PicaComic] 漫画 %s/%d 直链发送失败", pcs.PcId, pcs.EpId)
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
					logPicaComic.Warn().
						Str("type", ctx.Event.MessageType).
						Msg("unsupported message type")
					ctx.SendMsgf("[PicaComic] 不支持的消息类型：%s", ctx.Event.MessageType)
					return
				}
				if err != nil {
					logPicaComic.Error().
						Err(err).
						Str("pcId", pcs.PcId).
						Int("epId", pcs.EpId).
						Msg("failed to upload pdf")
					ctx.SendMsgf("[PicaComic] 漫画 %s/%d pdf上传失败：%v", pcs.PcId, pcs.EpId, err)
					return
				}
				ctx.SendMsgReplyf("[PicaComic] 漫画 %s/%d pdf上传完毕！\n(%s)", pcs.PcId, pcs.EpId, time.Since(ts))
			}

		}

	}
}

// 打包回调
func picaComicPackFuncWraper(f func(segChain message.SegmentArray)) func([]pc.Image) {
	return func(batch []pc.Image) {
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

var picaComicLock Locker[PicaComicSubmatch]

type PicaComicParseOption struct {
	DlAll        bool // 绕过 [MangaConfig.MaxForwardImages] 限制
	ToJpg        bool
	Salt         bool // 加盐修改哈希
	ToPdf        bool // 转为 pdf, 此时需要jpg且不加盐
	Purge        bool // 无视缓存
	DownloadOnly bool

	SliceSyntaxUsed bool
}

type PicaComicParse struct {
	Pcs     []PicaComicSubmatch
	Details map[string]*pc.ComicInfoResp
	Eps     map[string]*pc.EpsResp
	Pages   map[PicaComicSubmatch]*pc.PagesResp

	Options PicaComicParseOption

	FetchCosts    map[PicaComicSubmatch]time.Duration
	DownloadCosts map[PicaComicSubmatch]time.Duration
	// UploadCosts map[PicaComicSubmatch]time.Duration

	Ctx context.Context
}

func NewPicaComicParse(ctx context.Context, pcs []PicaComicSubmatch, sss []slicesyntax.SliceSyntaxes, options PicaComicParseOption) (pp *PicaComicParse, err error) {
	pp = &PicaComicParse{
		Pcs:     pcs,
		Details: make(map[string]*pc.ComicInfoResp, len(pcs)),
		Eps:     make(map[string]*pc.EpsResp, len(pcs)),
		Pages:   make(map[PicaComicSubmatch]*pc.PagesResp, len(pcs)),

		Options: options,

		FetchCosts:    make(map[PicaComicSubmatch]time.Duration, len(pcs)),
		DownloadCosts: make(map[PicaComicSubmatch]time.Duration, len(pcs)),
		// UploadCosts: make(map[PicaComicSubmatch]time.Duration, len(pcIds)),

		Ctx: ctx,
	}

	// 获取漫画数据 同时解析 [slicesyntax.SliceSyntaxes]
	wg := sync.WaitGroup{}
	for i, pcs := range pp.Pcs {
		tStart := time.Now()

		var detail *pc.ComicInfoResp
		var eps *pc.EpsResp
		var pages *pc.PagesResp
		var errD, errE, errP error

		wg.Go(func() {
			if pp.Details[pcs.PcId] == nil {
				detail, errD = pc.ComicInfo(pp.Ctx, pcs.PcId)
			}
		})
		wg.Go(func() {
			if pp.Eps[pcs.PcId] == nil {
				eps, errE = pc.Episodes(pp.Ctx, pcs.PcId, -1)
			}
		})
		wg.Go(func() { pages, errP = pc.Pages(pp.Ctx, pcs.PcId, pcs.EpId, -1) })
		wg.Wait()

		if errD != nil {
			return nil, errD
		}
		if errE != nil {
			return nil, errE
		}
		if errP != nil {
			return nil, errP
		}

		if sss[i] != nil {
			options.SliceSyntaxUsed = true
			pages.Pages.Docs = slicesyntax.DoIndexes(pages.Pages.Docs, sss[i].ToIndexesNoRepeat(len(pages.Pages.Docs)))
		}
		if !pp.Options.DlAll {
			if pp.Options.ToPdf {
				if len(pages.Pages.Docs) > mangaConfig.MaxPdfImages {
					pages.Pages.Docs = pages.Pages.Docs[:mangaConfig.MaxPdfImages]
				}
			} else {
				if len(pages.Pages.Docs) > mangaConfig.MaxForwardImages {
					pages.Pages.Docs = pages.Pages.Docs[:mangaConfig.MaxForwardImages]
				}
			}
		}
		pp.Details[pcs.PcId] = detail
		pp.Eps[pcs.PcId] = eps
		pp.Pages[pcs] = pages

		pp.FetchCosts[pcs] = time.Since(tStart)
	}

	return pp, nil
}

func (pp *PicaComicParse) DownloadingHint(i int) string {
	pcs := pp.Pcs[i]
	if len(pp.Pcs) == 1 {
		return fmt.Sprintf("漫画 %s/%d 获取耗时%s\n下载中...", pcs.PcId, pcs.EpId, pp.FetchCosts[pcs])
	}
	return fmt.Sprintf("漫画 %s/%d 获取耗时%s\n下载中...(%d/%d)", pcs.PcId, pcs.EpId, pp.FetchCosts[pcs], i+1, len(pp.Pcs))
}

func (pp *PicaComicParse) SendingHint(i int) string {
	pcs := pp.Pcs[i]
	if len(pp.Pcs) == 1 {
		return fmt.Sprintf("漫画 %s/%d 下载耗时%s\n发送中...", pcs.PcId, pcs.EpId, pp.DownloadCosts[pcs])
	}
	return fmt.Sprintf("漫画 %s/%d 下载耗时%s\n发送中...(%d/%d)", pcs.PcId, pcs.EpId, pp.DownloadCosts[pcs], i+1, len(pp.Pcs))
}

func (pp *PicaComicParse) ComicHeader(i int) (header, chapters message.SegmentArray) {
	pcs := pp.Pcs[i]
	comic := pp.Details[pcs.PcId].Comic
	eps := pp.Eps[pcs.PcId].Eps.Docs
	cUrl := fmt.Sprintf("pica://%s", comic.Id)

	if !pp.Options.ToPdf {
		header.Append(message.Text("一分钟后撤回，提前转发走\n"))
	}

	var comicFin string
	if comic.Finished {
		comicFin = " (完)"
	}
	header.Append(message.Textf(
		`%s%s
%s
%d❤️ | %dP
%s`,
		comic.Title, comicFin,
		strings.Join(comic.Categories, "，"),
		comic.LikesCount, comic.PagesCount,
		cUrl,
	))

	if !pp.Options.DlAll {
		limit := 0
		if pp.Options.ToPdf {
			limit = mangaConfig.MaxPdfImages
		} else {
			limit = mangaConfig.MaxForwardImages
		}
		header.Append(message.Textf("\n只发送最多%d页", limit))
		if !pp.Options.SliceSyntaxUsed {
			start := limit
			end := min(comic.PagesCount, limit*2)
			header.Append(message.Textf(
				"，发送 \"%s [%d:%d]\" 获取下一批",
				cUrl, start, end,
			))
		}
	}

	if len(eps) != 0 {
		sb := strings.Builder{}
		sb.WriteString("所有章節：")
		for _, ep := range eps {
			fmt.Fprintf(
				&sb,
				"\n%s：pica://%s/%d",
				ep.Title, pcs.PcId, ep.Order,
			)
		}
		chapters = message.SegmentArray{message.Text(sb.String())}
	}

	return
}

func (pp *PicaComicParse) DownloadIter(i int) iter.Seq2[pc.Image, error] {
	return func(yield func(pc.Image, error) bool) {
		pcId := pp.Pcs[i]
		pages := pp.Pages[pcId]
		for img, err := range pc.DownloadPagesIter(pp.Ctx, pages) {
			if err != nil {
				if yield(img, err) {
					continue
				}
				return
			}

			if pp.Options.ToJpg {
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

			if pp.Options.Salt {
				img.Data = salt(img.Data, mangaConfig.SaltLength)
			}

			if !yield(img, nil) {
				return
			}
		}
	}
}

func (pp *PicaComicParse) BuildForward(i int, uid int, name string) (n int, forward message.SegmentArray, err error) {
	pcs := pp.Pcs[i]
	var bp *utils.BatchPacker[pc.Image]
	if !pp.Options.DownloadOnly {
		bp = utils.NewBatchPacker(mangaConfig.ForwardMsgBatchSize,
			picaComicPackFuncWraper(func(segChain message.SegmentArray) {
				forward.Append(message.Node3(uid, name, segChain))
			}),
		)
	}

	tStart := time.Now()
	for img, err := range pp.DownloadIter(i) {
		if err != nil {
			return n, nil, fmt.Errorf("comic %s/%d failed to download image %s: %w", pcs.PcId, pcs.EpId, img.Ii.OriginalName, err)
		}

		n++

		if !pp.Options.DownloadOnly {
			bp.Append(img)
		}
	}
	pp.DownloadCosts[pcs] = time.Since(tStart)

	if !pp.Options.DownloadOnly {
		bp.Pack()
	}

	return n, forward, nil
}

func (pp *PicaComicParse) BuildPdf(i int, filename, filepath string) (n int, err error) {
	if pp.Options.Purge {
		_ = os.Remove(filepath)
	}
	_, err = os.Stat(filepath)
	if err != nil {
		pcs := pp.Pcs[i]
		const comicUrl = ""

		pdf := NewImagePdf()
		defer pdf.Close()

		tStart := time.Now()
		for img, err := range pp.DownloadIter(i) {
			if err != nil {
				return n, fmt.Errorf("comic %s/%d failed to download image %s: %w", pcs.PcId, pcs.EpId, img.Ii.OriginalName, err)
			}

			jpg, bounds, err := imgToJpg(img.Data)
			if err != nil {
				return n, fmt.Errorf("comic %s/%d failed to convert image %s to jpg: %w", pcs.PcId, pcs.EpId, img.Ii.OriginalName, err)
			}
			pdf.AddImage(jpg, bounds, comicUrl)

			n++
		}
		pp.DownloadCosts[pcs] = time.Since(tStart)

		err = pdf.OutputFileAndClose(filepath)
		if err != nil {
			return n, fmt.Errorf("comic %s/%d failed to create pdf: %w", pcs.PcId, pcs.EpId, err)
		}

		// 调用 qpdf 将 pdf 线性化
		out, err := exec.Command("qpdf", filepath, "--linearize", "--replace-input").CombinedOutput()
		if err != nil {
			logPicaComic.Warn().
				Err(err).
				Str("pcId", pcs.PcId).
				Int("epId", pcs.EpId).
				Str("output", string(out)).
				Msg("failed to linearize pdf")
		}
	}
	return
}
