package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"NothingBot_v4/ocrspace"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

type OcrConfig struct {
	Ocrspace struct {
		ApiKey          string
		DefaultLanguage string
		OCREngine       int
		IsTable         bool
	}
	// [TODO] other OCR providers
}

const ocrMId ModuleId = "Ocr"

var (
	ocrConfig      = OcrConfig{}
	ocrspaceConfig *ocrspace.Config
)

var moduleOcr = Module{
	ModuleMeta: ModuleMeta{
		Name:       ocrMId,
		Desc:       "图片OCR，支持多张",
		Conditions: Conditions{REMARK_REPLY | REMARK_WITH_IMAGE, REMARK_WITH_IMAGE},
		HelpMsg:    "--ocr",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_Ocr.go")

	moduleOcr.Init = initOcr
	moduleOcr.ReInit = initOcr
	modules.Add(&moduleOcr)
}

func initOcr() {
	err := config.DecodeModule(ocrMId, &ocrConfig)
	if err != nil {
		log.Error(err)
		return
	}

	ocrEngine := ocrConfig.Ocrspace.OCREngine
	if ocrEngine != 1 && ocrEngine != 2 {
		ocrEngine = 2
	}
	ocrspaceConfig, err = ocrspace.NewConfig(
		ocrspace.WithApiKey(ocrConfig.Ocrspace.ApiKey),
		ocrspace.WithLanguage(ocrConfig.Ocrspace.DefaultLanguage),
		ocrspace.WithOCREngine(ocrEngine),
		ocrspace.WithIsTable(ocrConfig.Ocrspace.IsTable),
	)
	if err != nil {
		log.Fatal("[OCR] failed to init ocrspace config:", err)
	}

	onebot.AddMatcher(moduleOcr.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnStringsContains("--ocr").
		Do(moduleOcr.RWMuWrap(ctxOcr)),
	)
}

func ctxOcr(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	msgSegs := ctx.ParsedSegments
	if ctx.ReplyId != 0 {
		resp, err := ctx.GetReplyMsg()
		if err != nil {
			ctx.SendMsgf("获取回复消息失败：%v", err)
			return
		}
		msgSegs = resp.Message
	}
	images := msgSegs.GetType(message.TYPE_IMAGE)
	if len(images) == 0 {
		ctx.SendMsgReply("消息中不存在图片")
		return
	}

	imgUrls := make([]string, 0, len(images))
	for _, image := range images {
		imgUrl, ok := image.Data["url"].(string)
		if !ok {
			continue
		}
		imgUrls = append(imgUrls, imgUrl)
	}

	if len(imgUrls) == 0 {
		ctx.SendMsgReply("无法从消息中提取图片")
		return
	} else if len(imgUrls) > 16 {
		ctx.SendMsgReply("图片太多啦，rurudo都看花眼了")
		return
	}

	forward := message.SegmentArray{}
	results := make(map[string][]string, len(imgUrls)) // url -> texts

	mu := sync.Mutex{}
	errs := make(chan error, 8)
	wg := sync.WaitGroup{}

	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	limiter := NewLimiter(8) // 最多 8 个并发
	defer limiter.Close()
	for _, imgUrl := range imgUrls {
		limiter.Acquire() <- struct{}{}
		wg.Add(1)
		go func() {
			defer limiter.Release()
			defer wg.Done()

			// 手动下载图片
			req, err := http.NewRequestWithContext(tctx, http.MethodGet, imgUrl, nil)
			if err != nil {
				errs <- fmt.Errorf("构建请求失败(%s): %w", imgUrl, err)
				cancel()
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs <- fmt.Errorf("下载失败(%s): %w", imgUrl, err)
				cancel()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				errs <- fmt.Errorf("下载非2xx(%s): %s", imgUrl, resp.Status)
				cancel()
				return
			}

			ocrResp, err := ocrspace.DoWithContext(tctx, resp.Body, ocrspaceConfig)
			if err != nil {
				errs <- fmt.Errorf("OCR失败(%s): %w", imgUrl, err)
				cancel()
				return
			}
			if ocrResp.IsErroredOnProcessing {
				errMsg, detail := ocrResp.Error()
				errs <- fmt.Errorf("OCR处理错误(%s): %s, %s", imgUrl, errMsg, detail)
				cancel()
				return
			}

			texts := ocrResp.Texts()
			mu.Lock()
			results[imgUrl] = texts
			mu.Unlock()
		}()
	}
	wg.Wait()

	// 汇总并回复
	if len(results) == 0 {
		err := make([]error, 0)
	LOOP:
		for {
			select {
			case e := <-errs:
				err = append(err, e)
			default:
				break LOOP
			}
		}
		if len(err) > 0 {
			ctx.SendMsgReply(fmt.Sprintf("OCR 失败，共 %d 个错误：%v", len(err), err[0]))
		} else {
			ctx.SendMsgReply("未识别到任何文本")
		}
		return
	}

	for url, lines := range results {
		if len(lines) == 0 {
			continue
		}
		forward.Append(message.Node3(uid, name, message.SegmentArray{
			message.Image(url),
			message.Text(strings.Join(lines, "\n")),
		}))
	}
	_, err := ctx.SendForwardMsgAuto(forward)
	if err != nil {
		ctx.SendMsg("结果合并转发发送失败")
		return
	}
}
