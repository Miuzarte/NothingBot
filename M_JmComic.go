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

	jm "github.com/Miuzarte/JMComic-go"
)

// 本文件的日志 scope
var logJmComic = logger.New("JmComic")

const (
	// [1]: site // unused
	// [2]: keyword
	JMCOMIC_SEARCH_REGEXP = `(?i)(JM|禁漫)搜索?[\s:：]*(.+)`

	// [1]: "jm" / url
	// [2]: jmId
	// [3]: slice syntax
	JMCOMIC_PARSE_REGEXP = `(?is)(JM|18comic\..+/.+/)(\d+)` +
		`\s*(\[(?:-?\d*:?)*\](?:\[(?:-?\d*:?)*\])*)?`
)

var (
	jmComicSearchReg = regexp.MustCompile(JMCOMIC_SEARCH_REGEXP)
	jmComicParseReg  = regexp.MustCompile(JMCOMIC_PARSE_REGEXP)
)

const jmComicMId ModuleId = "JmComic"

var (
	moduleJmComicSearch = ModuleMeta{
		Name:       jmComicMId.WithSuffix("Search"),
		Desc:       "禁漫天堂搜索",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg:    JMCOMIC_SEARCH_REGEXP,
	}
	moduleJmComicParse = ModuleMeta{
		Name:       jmComicMId.WithSuffix("Parse"),
		Desc:       "禁漫天堂解析",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg: JMCOMIC_PARSE_REGEXP +
			"\nNote：纯JMID需@bot" +
			"\n\n参数：" +
			"\n--pdf 合并为 Pdf 文件发送" +
			"\n--noupload 不上传 Pdf 至文件",
	}
)

var moduleJmComic = Module{
	ModuleMeta: ModuleMeta{
		Name:   jmComicMId,
		Hidden: true,
	},
	Priority: 1, // after [moduleManga]
	Disable:  env.Testing,
	SubModules: []*ModuleMeta{
		&moduleJmComicSearch,
		&moduleJmComicParse,
	},
}

func init() {
	NoBuildPrintFile("M_JmComic.go")

	moduleJmComic.Init = initJmComic
	moduleJmComic.ReInit = initJmComic
	modules.Add(&moduleJmComic)
}

func initJmComic() {
	if mangaConfig.Threads > 0 {
		jm.SetThreads(mangaConfig.Threads)
	}
	jm.SetUseEnvProxy(mangaConfig.UseEnvProxy)

	onebot.AddMatcher(moduleJmComicSearch.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return mangaConfig.JmComicEnabled && mangaConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(jmComicSearchReg).
		Do(moduleJmComic.RWMuWrap(ctxJmComicSearch)),
	)
	onebot.AddMatcher(moduleJmComicParse.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return mangaConfig.JmComicEnabled && mangaConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(jmComicParseReg).
		Do(moduleJmComic.RWMuWrap(ctxJmComicParse)),
	)
}

func ctxJmComicSearch(ctx *EasyOnebot.Ctx) {
	submatch := ctx.Submatches.Get(moduleJmComicSearch.Name.String())[0]
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))

	keyword := submatch[2]

	respSearch, err := ctx.SendMsg("[JmComic] 搜索中...")
	if err != nil {
		logJmComic.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	ms := NewMangaSearch(ctx, tctx, MANGA_SITE_JM, keyword)
	forward, err := ms.Do()
	if err != nil {
		ctx.SendMsgf("[JmComic] 搜索失败：%v", err)
		return
	}

	ts := time.Now()
	respSendForward, err := ctx.SendForwardMsgAuto(forward)
	ctx.DeleteMsg(respSearch.MessageID)
	if err != nil {
		ctx.SendMsg("[JmComic] 搜索结果发送失败")
		return
	}
	respRecallHint, _ := ctx.SendMsgReplyf("[JmComic] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

	if !pNoRecall {
		registerTimerRecall(respSendForward.MessageID)
		if respRecallHint != nil {
			registerTimerRecall(respRecallHint.MessageID)
		}
	}
}

type JmComicSubmatch struct {
	Method string // "jm" | "18comic.vip/xxxxx/"
	JmId   int
}

type JmComicSubmatches []JmComicSubmatch

func (jcs JmComicSubmatches) NeedConfirm() bool {
	// 只有单 jmId 时需要确认
	return len(jcs) == 1 && jcs[0].Method == "jm"
}

func (jcs JmComicSubmatches) JmIds() []int {
	ids := make([]int, 0, len(jcs))
	for _, jc := range jcs {
		ids = append(ids, jc.JmId)
	}
	return ids
}

func ctxJmComicParse(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatches := ctx.Submatches.Get(moduleJmComicParse.Name.String())
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pUsePdf := strings.Contains(lowerRm, "--pdf")
	pNoUpload := strings.Contains(lowerRm, "--noupload") || strings.Contains(lowerRm, "--no-upload")
	pPurge := strings.Contains(lowerRm, "--purge")
	pDownload := strings.Contains(lowerRm, "--download") // 仅下载
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))
	pAll := strings.Contains(lowerRm, "--all") && ctx.IsSuperuser

	jcs := make(JmComicSubmatches, 0, len(submatches))
	sss := make([]slicesyntax.SliceSyntaxes, len(submatches))
	for i, submatch := range submatches {
		jmId, err := strconv.Atoi(submatch[2])
		if err != nil {
			ctx.SendMsgf("[JmComic] [FIXME] unexpected non-numerical jmId: %s", submatch[2])
			return
		}
		jcs = append(jcs, JmComicSubmatch{
			strings.ToLower(submatch[1]),
			jmId,
		})
		if submatch[3] != "" {
			sss[i] = slicesyntax.ParseMulti(submatch[2])
		}
	}

	if len(jcs) > 2 && !ctx.IsSuperuser {
		ctx.SendMsgf("[JmComic] ☝️哒咩！%d个漫画太多了", len(jcs))
		return
	}

	if jcs.NeedConfirm() && !ctx.IsToMe {
		if len(ctx.ParsedSegments.GetType(message.TYPE_IMAGE)) == 0 {
			ctx.SendMsgf("[JmComic] 要解析JM%d请at我！", jcs[0].JmId)
		}
		return
	}

	respFetch, err := ctx.SendMsg("[JmComic] 获取中...")
	if err != nil {
		logJmComic.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	jpOp := JmComicParseOption{
		DlAll:        pAll,
		ToPdf:        pUsePdf,
		Purge:        pPurge,
		DownloadOnly: pDownload,
	}
	if jpOp.ToPdf {
		jpOp.ToJpg = true
		jpOp.Salt = false
	} else {
		jpOp.ToJpg = false
		jpOp.Salt = true
	}
	jp, err := NewJmComicParse(tctx, jcs.JmIds(), sss, jpOp)
	if err != nil {
		ctx.SendMsgReplyf("[JmComic] 解析失败：%v", err)
		return
	}

	failedJmIds := jmComicLock.TryLock(jp.JmIds)
	if len(failedJmIds) > 0 {
		ctx.SendMsgReplyf("[JmComic] locks of comic(s) %v are holding by other users", failedJmIds)
		return
	}
	defer jmComicLock.Unlock(jp.JmIds)

	// 每个漫画发一个合并转发
	for i, jmId := range jp.JmIds {
		respDownload, err := ctx.SendMsg(jp.DownloadingHint(i))
		if i == 0 {
			ctx.DeleteMsg(respFetch.MessageID)
		}
		if err != nil {
			logJmComic.Error().
				Err(err).
				Msg("failed to send msg")
			return
		}

		header, chapters := jp.ComicHeader(i)
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
			n, nodes, err := jp.BuildForward(i, uid, name)
			if err != nil {
				ctx.SendMsgf("[JmComic] %v", err)
				return
			}

			if pDownload {
				_, err := ctx.SendMsgReplyf("[JmComic] JM%d 下载完成 (%d)", jmId, n)
				if err != nil {
					logJmComic.Error().
						Err(err).
						Msg("failed to send msg")
				}
				continue
			}

			respSend, err := ctx.SendMsg(jp.SendingHint(i))
			ctx.DeleteMsg(respDownload.MessageID)
			if err != nil {
				logJmComic.Error().
					Err(err).
					Msg("failed to send msg")
				return
			}

			ts := time.Now()
			respSendForward, err := ctx.SendForwardMsgAuto(append(forward, nodes...))
			ctx.DeleteMsg(respSend.MessageID)
			if err != nil {
				ctx.SendMsgf("[JmComic] JM%d 发送失败", jmId)
				return
			}
			respRecallHint, _ := ctx.SendMsgReplyf("[JmComic] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

			if !pNoRecall {
				registerTimerRecall(respSendForward.MessageID)
				if respRecallHint != nil {
					registerTimerRecall(respRecallHint.MessageID)
				}
			}

		} else {
			filename := fmt.Sprintf("%d.pdf", jmId)
			filepath := filepath.Join(env.WorkDir, mangaConfig.JmComicCacheDir, filename)
			_, err := jp.BuildPdf(i, filename, filepath)
			if err != nil {
				ctx.SendMsgf("[JmComic] %v", err)
				return
			}

			// 发送漫画信息
			_, err = ctx.SendForwardMsgAuto(forward)
			if err != nil {
				logJmComic.Error().
					Err(err).
					Int("jmId", jmId).
					Msg("failed to send msg")
				ctx.SendMsgf("[JmComic] JM%d 信息合并转发发送失败", jmId)
			}

			// 发送直链
			_, err = ctx.SendMsgf("[JmComic] 直接查看：https://jmcomic.miuzarte.top/%s", filename)
			if err != nil {
				logJmComic.Error().
					Err(err).
					Int("jmId", jmId).
					Msg("failed to send msg")
				ctx.SendMsgf("[JmComic] JM%d 直链发送失败", jmId)
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
					logJmComic.Warn().
						Str("type", ctx.Event.MessageType).
						Msg("unsupported message type")
					ctx.SendMsgf("[JmComic] 不支持的消息类型：%s", ctx.Event.MessageType)
					return
				}
				if err != nil {
					logJmComic.Error().
						Err(err).
						Int("jmId", jmId).
						Msg("failed to upload pdf")
					ctx.SendMsgf("[JmComic] JM%d pdf上传失败：%v", jmId, err)
					return
				}
				ctx.SendMsgReplyf("[JmComic] JM%d pdf上传完毕！\n(%s)", jmId, time.Since(ts))
			}

		}

	}
}

// 打包回调
func jmComicPackFuncWraper(f func(segChain message.SegmentArray)) func([]jm.Image) {
	return func(batch []jm.Image) {
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

var jmComicLock Locker[int]

type JmComicParseOption struct {
	DlAll        bool // 绕过 [MangaConfig.MaxForwardImages] 限制
	ToJpg        bool
	Salt         bool // 加盐修改哈希
	ToPdf        bool // 转为 pdf, 此时需要jpg且不加盐
	Purge        bool // 无视缓存
	DownloadOnly bool

	SliceSyntaxUsed bool
}

type JmComicParse struct {
	JmIds    []int
	Albums   map[int]*jm.Album
	Chapters map[int]*jm.Chapter

	Options JmComicParseOption

	FetchCosts    map[int]time.Duration
	DownloadCosts map[int]time.Duration
	// UploadCosts map[int]time.Duration

	Ctx context.Context
}

func NewJmComicParse(ctx context.Context, jmIds []int, sss []slicesyntax.SliceSyntaxes, options JmComicParseOption) (jp *JmComicParse, err error) {
	jp = &JmComicParse{
		JmIds:    jmIds,
		Albums:   make(map[int]*jm.Album, len(jmIds)),
		Chapters: make(map[int]*jm.Chapter, len(jmIds)),

		Options: options,

		FetchCosts:    make(map[int]time.Duration, len(jmIds)),
		DownloadCosts: make(map[int]time.Duration, len(jmIds)),
		// UploadCosts: make(map[int]time.Duration, len(jmIds)),

		Ctx: ctx,
	}

	// 获取漫画数据 同时解析 [slicesyntax.SliceSyntaxes]
	wg := sync.WaitGroup{}
	for i, jmId := range jp.JmIds {
		tStart := time.Now()

		var album *jm.Album
		var chapter *jm.Chapter
		var errA, errC error

		wg.Go(func() { album, errA = jm.GetAlbum(jp.Ctx, jmId) })
		wg.Go(func() { chapter, errC = jm.GetChapter(jp.Ctx, jmId) })
		wg.Wait()

		if errA != nil {
			return nil, errA
		}
		if errC != nil {
			return nil, errC
		}

		if sss[i] != nil {
			options.SliceSyntaxUsed = true
			chapter.Images = slicesyntax.DoIndexes(chapter.Images, sss[i].ToIndexesNoRepeat(len(chapter.Images)))
		}
		if !jp.Options.DlAll {
			if jp.Options.ToPdf {
				if len(chapter.Images) > mangaConfig.MaxPdfImages {
					chapter.Images = chapter.Images[:mangaConfig.MaxPdfImages]
				}
			} else {
				if len(chapter.Images) > mangaConfig.MaxForwardImages {
					chapter.Images = chapter.Images[:mangaConfig.MaxForwardImages]
				}
			}
		}
		jp.Albums[jmId] = album
		jp.Chapters[jmId] = chapter

		jp.FetchCosts[jmId] = time.Since(tStart)
	}

	return jp, nil
}

func (jp *JmComicParse) DownloadingHint(i int) string {
	jmId := jp.JmIds[i]
	if len(jp.JmIds) == 1 {
		return fmt.Sprintf("JM%d 获取耗时%s\n下载中...", jmId, jp.FetchCosts[jmId])
	}
	return fmt.Sprintf("JM%d 获取耗时%s\n下载中...(%d/%d)", jmId, jp.FetchCosts[jmId], i+1, len(jp.JmIds))
}

func (jp *JmComicParse) SendingHint(i int) string {
	jmId := jp.JmIds[i]
	if len(jp.JmIds) == 1 {
		return fmt.Sprintf("JM%d 下载耗时%s\n发送中...", jmId, jp.DownloadCosts[jmId])
	}
	return fmt.Sprintf("JM%d 下载耗时%s\n发送中...(%d/%d)", jmId, jp.DownloadCosts[jmId], i+1, len(jp.JmIds))
}

func (jp *JmComicParse) ComicHeader(i int) (header, chapters message.SegmentArray) {
	jmId := jp.JmIds[i]
	album := jp.Albums[jmId]
	series := album.Series
	chapter := jp.Chapters[jmId]
	cUrl := fmt.Sprintf("18comic.vip/album/%d", album.Id)

	if !jp.Options.ToPdf {
		header.Append(message.Text("一分钟后撤回，提前转发走\n"))
	}

	header.Append(message.Textf(
		`%s
作品：%s
人物：%s
标签：%s
%d 阅读 | %d 点赞
%s`,
		album.Name,
		strings.Join(album.Works, "，"),
		strings.Join(album.Actors, "，"),
		strings.Join(album.Tags, "，"),
		album.TotalViews, album.Likes,
		cUrl,
	))

	if !jp.Options.DlAll {
		limit := 0
		if jp.Options.ToPdf {
			limit = mangaConfig.MaxPdfImages
		} else {
			limit = mangaConfig.MaxForwardImages
		}
		header.Append(message.Textf("\n只发送最多%d页", limit))
		if !jp.Options.SliceSyntaxUsed {
			start := limit
			end := min(len(chapter.Images), limit*2)
			header.Append(message.Textf(
				"，发送 \"%s [%d:%d]\" 获取下一批",
				cUrl, start, end,
			))
		}
	}

	if len(series) != 0 {
		sb := strings.Builder{}
		sb.WriteString("所有章节：")
		for i, serie := range series {
			if serie.Name != "" {
				fmt.Fprintf(
					&sb,
					"\n第%d话（%s）：JM%s",
					i+1, serie.Name, serie.Id,
				)
			} else {
				fmt.Fprintf(
					&sb,
					"\n第%d话：JM%s",
					i+1, serie.Id,
				)
			}
		}
		chapters = message.SegmentArray{message.Text(sb.String())}
	}

	return
}

func (jp *JmComicParse) DownloadIter(i int) iter.Seq2[jm.Image, error] {
	return func(yield func(jm.Image, error) bool) {
		jmId := jp.JmIds[i]
		chapter := jp.Chapters[jmId]
		for img, err := range jm.DownloadComicIter(jp.Ctx, chapter) {
			if err != nil {
				if yield(img, err) {
					continue
				}
				return
			}

			if jp.Options.ToJpg {
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

			if jp.Options.Salt {
				img.Data = salt(img.Data, mangaConfig.SaltLength)
			}

			if !yield(img, nil) {
				return
			}
		}
	}
}

func (jp *JmComicParse) BuildForward(i int, uid int, name string) (n int, forward message.SegmentArray, err error) {
	jmId := jp.JmIds[i]
	var bp *utils.BatchPacker[jm.Image]
	if !jp.Options.DownloadOnly {
		bp = utils.NewBatchPacker(mangaConfig.ForwardMsgBatchSize,
			jmComicPackFuncWraper(func(segChain message.SegmentArray) {
				forward.Append(message.Node3(uid, name, segChain))
			}),
		)
	}

	tStart := time.Now()
	for img, err := range jp.DownloadIter(i) {
		if err != nil {
			return n, nil, fmt.Errorf("JM%d failed to download image %s: %w", jmId, img.Name, err)
		}

		n++

		if !jp.Options.DownloadOnly {
			bp.Append(img)
		}
	}
	jp.DownloadCosts[jmId] = time.Since(tStart)

	if !jp.Options.DownloadOnly {
		bp.Pack()
	}

	return n, forward, nil
}

func (jp *JmComicParse) BuildPdf(i int, filename, filepath string) (n int, err error) {
	if jp.Options.Purge {
		_ = os.Remove(filepath)
	}
	_, err = os.Stat(filepath)
	if err != nil {
		jmId := jp.JmIds[i]
		comicUrl := fmt.Sprintf("https://18comic.vip/photo/%d", jmId)

		pdf := NewImagePdf()
		defer pdf.Close()

		tStart := time.Now()
		for img, err := range jp.DownloadIter(i) {
			if err != nil {
				return n, fmt.Errorf("JM%d failed to download image %s: %w", jmId, img.Name, err)
			}

			jpg, bounds, err := imgToJpg(img.Data)
			if err != nil {
				return n, fmt.Errorf("JM%d failed to convert image %s to jpg: %w", jmId, img.Name, err)
			}
			pdf.AddImage(jpg, bounds, comicUrl)

			n++
		}
		jp.DownloadCosts[jmId] = time.Since(tStart)

		err = pdf.OutputFileAndClose(filepath)
		if err != nil {
			return n, fmt.Errorf("JM%d failed to create pdf: %w", jmId, err)
		}

		// 调用 qpdf 将 pdf 线性化
		out, err := exec.Command("qpdf", filepath, "--linearize", "--replace-input").CombinedOutput()
		if err != nil {
			logJmComic.Warn().
				Err(err).
				Int("jmId", jmId).
				Str("output", string(out)).
				Msg("failed to linearize pdf")
		}
	}
	return
}
