package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"
	"NothingBot_v4/slicesyntax"
	"NothingBot_v4/utils"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/everpcpc/pixiv"
)

// 本文件的日志 scope
var logPixiv = logger.New("Pixiv")

// [1]: pid
// [2]: slice syntax
const PIXIV_PARSE_REGEXP = `(?i)(?:(?:pixiv\.net/(?:i|artworks)/|(?:看看|kk)(?:批|pixiv|p站|p)\s*)(\d+))` +
	`\s*(\[(?:-?\d*:?)*\](?:\[(?:-?\d*:?)*\])*)?`

var pixivParseReg = regexp.MustCompile(PIXIV_PARSE_REGEXP)

type PixivConfig struct {
	Enabled             bool
	SaltLength          int
	MaxForwardImages    int
	ForwardMsgBatchSize int
	MaxPdfImages        int
	Threads             int
	AccessToken         string
	RefreshToken        string
	CacheDir            string
	List
}

const pixivMId ModuleId = "PixivParse"

var (
	pixivConfig = PixivConfig{}
	pixivClient = pixiv.NewApp()
	// pixivCache  = PixivCache{}
)

var modulePixiv = Module{
	ModuleMeta: ModuleMeta{
		Name:       pixivMId,
		Desc:       "Pixiv链接解析",
		Conditions: Conditions{REMARK_WHITE_LIST},
		HelpMsg: PIXIV_PARSE_REGEXP +
			"\n\n参数：" +
			"\n--pdf 合并为 Pdf 文件" +
			"\n--noupload 不上传 Pdf 至文件",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_Pixiv.go")

	modulePixiv.Init = initPixivParse
	modulePixiv.ReInit = initPixivParse
	modules.Add(&modulePixiv)
}

func initPixivParse() {
	err := config.DecodeModule(pixivMId, &pixivConfig)
	if err != nil {
		logPixiv.Error().
			Err(err).
			Msg("failed to decode config")
		return
	}

	if !pixivConfig.Enabled {
		return
	}

	if pixivConfig.AccessToken == "" || pixivConfig.RefreshToken == "" {
		logPixiv.Error().Msg("access token or refresh token not set")
		return
	}

	pixivClient.SetThreads(pixivConfig.Threads)

	_, err = pixiv.LoadAuth(pixivConfig.AccessToken, pixivConfig.RefreshToken, time.Time{})
	if err != nil {
		logPixiv.Error().
			Err(err).
			Msg("failed to load auth")
		return
	}

	// if pixivCache.Root == nil { // 更新需要重启
	// 	err := pixivCache.Init(pixivConfig.CacheDir)
	// 	if err != nil {
	// 		logPixiv.Error().Err(err).Msg("failed to init cache")
	// 		return
	// 	}
	// }

	onebot.AddMatcher(modulePixiv.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnlyType(message.TYPE_TEXT).
		OnFunc(func(ctx *EasyOnebot.Ctx) bool {
			return pixivConfig.Enabled && pixivConfig.White(ctx)
		}).
		OnRegexpFindAllStringSubmatch(pixivParseReg).
		Do(modulePixiv.RWMuWrap(ctxPixivParse)),
	)
}

func ctxPixivParse(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	submatches := ctx.Submatches.Get(modulePixiv.Name.String())
	lowerRm := strings.ToLower(ctx.Event.RawMessage)
	pUsePdf := strings.Contains(lowerRm, "--pdf")
	pNoUpload := strings.Contains(lowerRm, "--noupload") || strings.Contains(lowerRm, "--no-upload")
	pPurge := strings.Contains(lowerRm, "--purge")
	pDownload := strings.Contains(lowerRm, "--download")
	pNoRecall := ctx.IsSuperuser && (strings.Contains(lowerRm, "--norecall") || strings.Contains(lowerRm, "--no-recall"))
	pAll := strings.Contains(lowerRm, "--all") && ctx.IsSuperuser

	pIds := make([]uint64, 0, len(submatches))
	sss := make([]slicesyntax.SliceSyntaxes, len(submatches))
	for i, submatch := range submatches {
		pId, _ := strconv.ParseUint(submatch[1], 10, 0)
		if pId != 0 {
			pIds = append(pIds, pId)
			if submatch[2] != "" {
				sss[i] = slicesyntax.ParseMulti(submatch[2])
			}
		}
	}

	if len(pIds) > 8 && !ctx.IsSuperuser {
		ctx.SendMsgf("[Pixiv] ☝️哒咩！%d个作品太多了", len(pIds))
		return
	}

	respFetch, err := ctx.SendMsg("[Pixiv] 获取中...")
	if err != nil {
		logPixiv.Error().
			Err(err).
			Msg("failed to send msg")
		return
	}

	tctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	ppOp := PixivParseOption{
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
	pp, err := NewPixivParse(tctx, pIds, sss, ppOp)
	if err != nil {
		ctx.SendMsgReplyf("[Pixiv] 解析失败：%v", err)
		return
	}

	failedPIds := pixivIllustLock.TryLock(pp.PIds)
	if len(failedPIds) > 0 {
		ctx.SendMsgReplyf("[Pixiv] locks of illust(s) %v are holding by other users", failedPIds)
		return
	}
	defer pixivIllustLock.Unlock(pp.PIds)

	// 每个作品发一个合并转发
	for i, pId := range pp.PIds {
		respDownload, err := ctx.SendMsg(pp.DownloadingHint(i))
		if i == 0 {
			ctx.Std.DeleteMsg(respFetch.MessageId)
		}
		if err != nil {
			logPixiv.Error().
				Err(err).
				Msg("failed to send msg")
			return
		}

		user, header := pp.IllustHeader(i)
		// 使用 pdf 时也用合并转发发送作品信息
		forward := message.SegmentArray{
			message.Node3(uid, name, user),
			message.Node3(uid, name, header),
		}

		if !pUsePdf {
			n, nodes, err := pp.BuildForward(i, uid, name)
			if err != nil {
				ctx.SendMsgf("[Pixiv] %v", err)
				return
			}

			if pDownload {
				_, err := ctx.SendMsgReplyf("[Pixiv] 作品 %d 下载完成 (%d)", pId, n)
				if err != nil {
					logPixiv.Error().
						Err(err).
						Msg("failed to send msg")
				}
				continue
			}

			respSend, err := ctx.SendMsg(pp.SendingHint(i))
			ctx.Std.DeleteMsg(respDownload.MessageId)
			if err != nil {
				logPixiv.Error().
					Err(err).
					Msg("failed to send msg")
				return
			}

			ts := time.Now()
			respSendForward, err := ctx.SendForwardMsgAuto(append(forward, nodes...))
			ctx.Std.DeleteMsg(respSend.MessageId)
			if err != nil {
				ctx.SendMsgf("[Pixiv] 作品 %d 发送失败", pId)
				return
			}
			respRecallHint, _ := ctx.SendMsgReplyf("[Pixiv] 请转发查收！一分钟后撤回\n(%s)", time.Since(ts))

			if !pNoRecall {
				registerTimerRecall(respSendForward.MessageId)
				if respRecallHint != nil {
					registerTimerRecall(respRecallHint.MessageId)
				}
			}

		} else {
			filename := fmt.Sprintf("%d.pdf", pId)
			filepath := filepath.Join(env.WorkDir, pixivConfig.CacheDir, filename)
			_, err := pp.BuildPdf(i, filename, filepath)
			if err != nil {
				ctx.SendMsgf("[Pixiv] %v", err)
				return
			}

			// 发送作品信息
			_, err = ctx.SendForwardMsgAuto(forward)
			if err != nil {
				logPixiv.Error().
					Err(err).
					Uint64("pId", pId).
					Msg("failed to send msg")
				ctx.SendMsgf("[Pixiv] 作品 %d 信息合并转发发送失败", pId)
			}

			// 发送直链
			_, err = ctx.SendMsgf("[Pixiv] 直接查看：https://pixiv.miuzarte.top/%s", filename)
			if err != nil {
				logPixiv.Error().
					Err(err).
					Uint64("pId", pId).
					Msg("failed to send msg")
				ctx.SendMsgf("[Pixiv] 作品 %d 直链发送失败", pId)
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
					logPixiv.Warn().
						Str("type", ctx.Event.MessageType).
						Msg("unsupported message type")
					ctx.SendMsgf("[Pixiv] 不支持的消息类型：%s", ctx.Event.MessageType)
					return
				}
				if err != nil {
					logPixiv.Error().
						Err(err).
						Uint64("pId", pId).
						Msg("failed to upload pdf")
					ctx.SendMsgf("[Pixiv] 作品 %d pdf上传失败：%v", pId, err)
					return
				}
				ctx.SendMsgReplyf("[Pixiv] 作品 %d pdf上传完毕！\n(%s)", pId, time.Since(ts))
			}

		}

	}
}

// 打包回调
func pixivPackFuncWraper(f func(segChain message.SegmentArray)) func([]pixiv.Image) {
	return func(batch []pixiv.Image) {
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

var pixivIllustLock Locker[uint64]

type PixivParseOption struct {
	DlAll        bool // 绕过 [PixivConfig.MaxForwardImages] 限制
	ToJpg        bool
	Salt         bool // 加盐修改哈希
	ToPdf        bool // 转为 pdf, 此时需要jpg且不加盐
	Purge        bool // 无视缓存
	DownloadOnly bool

	SliceSyntaxUsed bool
}

type PixivParse struct {
	PIds    []uint64
	Illusts map[uint64]*pixiv.Illust

	Options PixivParseOption

	FetchCosts    map[uint64]time.Duration
	DownloadCosts map[uint64]time.Duration
	// UploadCosts map[uint64]time.Duration

	Ctx context.Context
}

func NewPixivParse(ctx context.Context, pIds []uint64, sss []slicesyntax.SliceSyntaxes, options PixivParseOption) (pp *PixivParse, err error) {
	pp = &PixivParse{
		PIds:    pIds,
		Illusts: make(map[uint64]*pixiv.Illust, len(pIds)),

		Options: options,

		FetchCosts:    make(map[uint64]time.Duration, len(pIds)),
		DownloadCosts: make(map[uint64]time.Duration, len(pIds)),
		// UploadCosts: make(map[uint64]time.Duration, len(pids)),

		Ctx: ctx,
	}

	// 获取作品数据 同时解析 [slicesyntax.SliceSyntaxes]
	for i, pId := range pp.PIds {
		tStart := time.Now()

		illust, err := pixivClient.IllustDetail(pId)
		if err != nil {
			return nil, err
		}

		if sss[i] != nil {
			options.SliceSyntaxUsed = true
			if illust.MetaSinglePage == nil || illust.MetaSinglePage.OriginalImageURL == "" {
				illust.MetaPages = slicesyntax.DoIndexes(illust.MetaPages, sss[i].ToIndexesNoRepeat(len(illust.MetaPages)))
			}
		}
		if !pp.Options.DlAll {
			if pp.Options.ToPdf {
				if len(illust.MetaPages) > mangaConfig.MaxPdfImages {
					illust.MetaPages = illust.MetaPages[:mangaConfig.MaxPdfImages]
				}
			} else {
				if len(illust.MetaPages) > mangaConfig.MaxForwardImages {
					illust.MetaPages = illust.MetaPages[:mangaConfig.MaxForwardImages]
				}
			}
		}
		pp.Illusts[illust.ID] = illust

		pp.FetchCosts[illust.ID] = time.Since(tStart)
	}

	return pp, nil
}

func (pp *PixivParse) DownloadingHint(i int) string {
	pId := pp.PIds[i]
	if len(pp.PIds) == 1 {
		return fmt.Sprintf("%d 获取耗时%s\n下载中...", pId, pp.FetchCosts[pId])
	}
	return fmt.Sprintf("%d 获取耗时%s\n下载中...(%d/%d)", pId, pp.FetchCosts[pId], i+1, len(pp.PIds))
}

func (pp *PixivParse) SendingHint(i int) string {
	pId := pp.PIds[i]
	if len(pp.PIds) == 1 {
		return fmt.Sprintf("%d 下载耗时%s\n发送中...", pId, pp.DownloadCosts[pId])
	}
	return fmt.Sprintf("%d 下载耗时%s\n发送中...(%d/%d)", pId, pp.DownloadCosts[pId], i+1, len(pp.PIds))
}

func (pp *PixivParse) IllustHeader(i int) (user, header message.SegmentArray) {
	pId := pp.PIds[i]
	illust := pp.Illusts[pId]

	tags := make([]string, 0, len(illust.Tags))
	for _, t := range illust.Tags {
		tags = append(tags, t.Name)
	}

	if !pp.Options.ToPdf {
		header.Append(message.Text("一分钟后撤回，提前转发走\n"))
	}

	user = message.SegmentArray{
		message.Image(strings.ReplaceAll(
			illust.User.ProfileImages.Medium,
			"i.pximg.net", "i.pixiv.cat",
		)),
		message.Textf(
			"%s\npixiv.net/u/%d",
			illust.User.Name,
			illust.User.ID,
		),
	}

	var note string
	if illust.IllustAIType == pixiv.IllustAITypeAIGenerated {
		note += "\n#[AI Generated]"
	}

	header = message.SegmentArray{
		message.Textf(
			`%s
%s
%s
%d浏览 %d收藏
%s
pixiv.net/i/%d
共%dP%s`,
			illust.Title,
			illust.Caption,
			strings.Join(tags, "，"),
			illust.TotalView, illust.TotalBookmarks,
			illust.CreateDate.Format("2006/01/02 15:04:05"),
			illust.ID,
			illust.PageCount,
			note,
		),
	}

	return
}

func (pp *PixivParse) DownloadIter(i int) iter.Seq2[pixiv.Image, error] {
	return func(yield func(pixiv.Image, error) bool) {
		pId := pp.PIds[i]
		illust := pp.Illusts[pId]
		for dr := range pixivClient.DownloadIllustIter(pp.Ctx, illust, pixiv.SIZE_ORIGINAL) {
			img, err := dr.Img, dr.Err
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
				img.Data = salt(img.Data, pixivConfig.SaltLength)
			}

			if !yield(img, nil) {
				return
			}
		}
	}
}

func (pp *PixivParse) BuildForward(i int, uid int, name string) (n int, forward message.SegmentArray, err error) {
	pId := pp.PIds[i]
	var bp *utils.BatchPacker[pixiv.Image]
	if !pp.Options.DownloadOnly {
		bp = utils.NewBatchPacker(mangaConfig.ForwardMsgBatchSize,
			pixivPackFuncWraper(func(segChain message.SegmentArray) {
				forward.Append(message.Node3(uid, name, segChain))
			}),
		)
	}

	tStart := time.Now()
	for img, err := range pp.DownloadIter(i) {
		if err != nil {
			return n, nil, fmt.Errorf("illust %d failed to download image %d: %w", pId, img.P, err)
		}

		n++

		if !pp.Options.DownloadOnly {
			bp.Append(img)
		}
	}
	pp.DownloadCosts[pId] = time.Since(tStart)

	if !pp.Options.DownloadOnly {
		bp.Pack()
	}

	return n, forward, nil
}

func (pp *PixivParse) BuildPdf(i int, filename, filepath string) (n int, err error) {
	if pp.Options.Purge {
		_ = os.Remove(filepath)
	}
	_, err = os.Stat(filepath)
	if err != nil {
		pId := pp.PIds[i]
		url := fmt.Sprintf("https://pixiv.net/i/%d", pId)

		pdf := NewImagePdf()
		defer pdf.Close()

		tStart := time.Now()
		for img, err := range pp.DownloadIter(i) {
			if err != nil {
				return n, fmt.Errorf("illust %d failed to download image %d: %w", pId, img.P, err)
			}

			jpg, bounds, err := imgToJpg(img.Data)
			if err != nil {
				return n, fmt.Errorf("illust %d failed to convert image %d to jpg: %w", pId, img.P, err)
			}
			pdf.AddImage(jpg, bounds, url)

			n++
		}
		pp.DownloadCosts[pId] = time.Since(tStart)

		err = pdf.OutputFileAndClose(filepath)
		if err != nil {
			return n, fmt.Errorf("illust %d failed to create pdf: %w", pId, err)
		}

		// 调用 qpdf 将 pdf 线性化
		out, err := exec.Command("qpdf", filepath, "--linearize", "--replace-input").CombinedOutput()
		if err != nil {
			logPixiv.Warn().
				Err(err).
				Uint64("pId", pId).
				Str("output", string(out)).
				Msg("failed to linearize pdf")
		}
	}
	return
}

const PIXIV_PARSE_CACHE_DIR_DEFAULT = "PixivCache"

// PixivCache stores "126763198/1", "126763198/2"...
// , "pid/p"
// , raw pic data as content, convert before sending
type PixivCache struct {
	Root *os.Root
}

func (pc *PixivCache) Init(path string) error {
	if path == "" {
		path = PIXIV_PARSE_CACHE_DIR_DEFAULT
	}
	var err error
	err = os.MkdirAll(path, 0o777)
	if err != nil {
		return err
	}
	pc.Root, err = os.OpenRoot(path)
	if err != nil {
		return err
	}
	return nil
}

func (pc *PixivCache) FetchOne(pid string, p int) (data []byte, err error) {
	path := filepath.Join(pid, Itoa(p))
	f, err := pc.Root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (pc *PixivCache) Fetch(pid string) (datas [][]byte, err error) {
	var fileCount int
	// [TODO] test [filepath.WalkDir]
	err = filepath.WalkDir(filepath.Join(pc.Root.Name(), pid), func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() != pid {
			return errors.New("unexpected dir") // no more subdir
		}
		if !info.IsDir() { // root dir self
			fileCount++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	datas = make([][]byte, fileCount)
	for i := range fileCount {
		datas[i], err = pc.FetchOne(pid, i+1)
		if err != nil {
			return nil, err
		}
	}
	return datas, nil
}

func (pc *PixivCache) Store(pid string, datas [][]byte) error {
	err := pc.Root.Mkdir(pid, 0o777) // 放在单独的文件夹下
	if err != nil {
		if os.IsExist(err) {
			err = nil
		} else {
			return err
		}
	}

	// root/pid/p
	for i, data := range datas {
		path := filepath.Join(pid, Itoa(i+1))
		fs, err := pc.Root.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			return err
		}
		defer fs.Close()
		_, err = fs.Write(data)
		if err != nil {
			return err
		}
	}

	return nil
}
