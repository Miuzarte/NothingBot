package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"NothingBot_v4/utils"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/message"

	a2d "github.com/Miuzarte/Ascii2d-go"
	sn "github.com/Miuzarte/SauceNAO-go"
	stb "github.com/Miuzarte/SoutuBot-go"
	"github.com/nfnt/resize"
)

type ReverseSearchSite int

const (
	_ ReverseSearchSite = iota
	REVERSE_SEARCH_SITE_SN
	REVERSE_SEARCH_SITE_A2D
	REVERSE_SEARCH_SITE_STB
)

func (rss ReverseSearchSite) String() string {
	switch rss {
	case REVERSE_SEARCH_SITE_SN:
		return "SauceNAO"
	case REVERSE_SEARCH_SITE_A2D:
		return "Ascii2d"
	case REVERSE_SEARCH_SITE_STB:
		return "SoutuBot"
	default:
		panic(fmt.Errorf("undefined: %d", rss))
	}
}

type ReverseSearch struct {
	Ctx                context.Context
	Site               ReverseSearchSite
	ImgSeg             message.Segment
	DownloadImgLocally bool

	NodeUserId   int
	NodeNickname string
}

func NewReverseSearch(ctx *EasyOnebot.Ctx, tctx context.Context, site ReverseSearchSite, imgSeg message.Segment, download bool) ReverseSearch {
	return ReverseSearch{
		Ctx:                tctx,
		Site:               site,
		ImgSeg:             imgSeg,
		DownloadImgLocally: download,

		NodeUserId:   ctx.Event.Sender.UserId,
		NodeNickname: ctx.Event.Sender.GetCardOrNickname(),
	}
}

func (rs *ReverseSearch) Do() (forward message.SegmentArray, err error, raw any) {
	var imgAny any
	if !rs.DownloadImgLocally {
		url, ok := rs.ImgSeg.Data["url"].(string)
		if !ok || !strings.HasPrefix(url, "http") {
			return nil, fmt.Errorf("获取图片链接失败：%v", rs.ImgSeg), nil
		}
		imgAny = url
	} else {
		body, err := rs.ImgSeg.Download(rs.Ctx)
		if err != nil {
			return nil, fmt.Errorf("图片下载失败：%w", err), nil
		}
		ct := http.DetectContentType(body)
		if !strings.HasPrefix(ct, "image/") {
			return nil, fmt.Errorf("错误的内容类型：%s", ct), nil
		}

		const resizeLimit = 4 * 1024 * 1024 // 4MiB
		if len(body) > resizeLimit {
			var newImgData []byte
			for _, longest := range [...]uint{1440, 1080, 720} {
				var err error
				newImgData, err = scaleDownImageJpeg(body, longest)
				if err != nil {
					return nil, fmt.Errorf("图片体积过大：%s, 图片缩放失败：%w", utils.FormatBytes(uint64(len(body))), err), nil
				}
				if len(newImgData) <= resizeLimit {
					break
				}
			}
			imgAny = newImgData
		} else {
			imgAny = body
		}
	}

	var resultsSegChains []message.SegmentArray
	switch rs.Site {
	case REVERSE_SEARCH_SITE_SN:
		resp, err := sauceNaoClient.Search(rs.Ctx, imgAny)
		if err != nil {
			return nil, err, nil
		}

		resultsSegChains = saucenaoBuildNodes(resp)
		raw = resp

	case REVERSE_SEARCH_SITE_A2D:
		c, b, err := ascii2dClient.Search(rs.Ctx, imgAny)
		if err != nil {
			return nil, err, nil
		}

		resultsSegChains = []message.SegmentArray{ascii2dBuildNode(rs.Ctx, c), ascii2dBuildNode(rs.Ctx, b)}
		raw = [2]a2d.Result{c, b}

	case REVERSE_SEARCH_SITE_STB:
		if !rs.DownloadImgLocally {
			panic("url is not supported in SoutuBot")
		}
		resp, err := soutuBotClient.Search(rs.Ctx, imgAny.([]byte))
		if err != nil {
			return nil, err, nil
		}

		resultsSegChains = soutubotBuildNodes(resp)
		raw = resp

	}

	forward = make(message.SegmentArray, 0, len(resultsSegChains))
	for _, segChain := range resultsSegChains {
		forward.Append(message.Node3(rs.NodeUserId, rs.NodeNickname, segChain))
	}

	return
}

func saucenaoBuildNodes(resp *sn.Response) []message.SegmentArray {
	segChains := make([]message.SegmentArray, 0, 1+len(resp.Results))
	for _, result := range resp.Results {
		header := result.Header
		data := result.DecodeData()

		v := reflect.ValueOf(data)
		if v.Kind() == reflect.Pointer && v.IsNil() {
			log.Warnf("[SauceNAO] todo response(%s): %s", header.IndexName, resp.RawBody)
			onebot.Log2Sus.Warnf("[SauceNAO] todo response(%s): %s", header.IndexName, resp.RawBody)
		}

		similarity, err := strconv.ParseFloat(header.Similarity, 64)
		hideThumbnail := sauceNaoConfig.HideWhenLowSim &&
			similarity < sauceNaoConfig.LowSimThreshold &&
			header.Hidden > 0

		segChain := make(message.SegmentArray, 0, 2)
		if err != nil {
			segChain.Append(message.Textf("解析匹配度失败(%#v): %v", header.Similarity, err))
		}
		if !hideThumbnail {
			segChain.Append(message.Image(header.Thumbnail))
		}
		segChain.Append(message.Textf(
			"SauceNAO (%s%%)\n%s\n%s",
			header.Similarity,
			header.IndexName,
			data,
		))

		segChains = append(segChains, segChain)
	}

	return segChains
}

func ascii2dBuildNode(ctx context.Context, res a2d.Result) message.SegmentArray {
	segChain := make(message.SegmentArray, 0, 4)

	var resultType string
	switch res.ResultType {
	case "color":
		resultType = "色合検索"
	case "bovw":
		resultType = "特徴検索"
	}

	imgData, err := ascii2dClient.Download(ctx, res)
	if err != nil {
		log.Warnf("[Ascii2d] 下载缩略图失败,回退到 URL: %v", err)
		segChain.Append(message.Image(res.Thumbnail))
	} else {
		segChain.Append(message.Image(imgData))
	}

	if res.IsRegisteredManually {
		segChain.Append(message.Text("登録された詳細\n"))
	}

	if res.Author != "" {
		segChain.Append(message.Textf("ascii2d %s\n%s - %s", resultType, res.Title, res.Author))
	} else {
		segChain.Append(message.Textf("ascii2d %s\n%s", resultType, res.Title))
	}

	if res.Url != "" {
		segChain.Append(message.Text("\n" + res.Url))
	}

	if res.AuthorUrl != "" {
		segChain.Append(message.Text("\n" + res.AuthorUrl))
	}

	return segChain
}

func soutubotBuildNodes(resp *stb.Response) []message.SegmentArray {
	segChains := make([]message.SegmentArray, 0, 1+len(resp.Results))
	params := resp.Query.RequestedParams
	head := message.SegmentArray{
		message.Textf("耗时 %.2fs\nfactor %.1f | top_k %d\n找到了 %d 条相似的结果",
			resp.Elapsed().Seconds(), params.Factor, params.TopK, len(resp.Results)),
	}
	if len(resp.Results) > 0 && resp.Results[0].Score < stb.MATCH_SIMILARITY_THRESHOLD {
		const lowSimHint = `
最大匹配度低于45，结果可能不正确
请自行判断，或更换严格模式/其他搜图引擎来搜索`
		head.Append(message.Text(lowSimHint))
	}
	head.Append(message.Textf("\n%s", resp.ResultUrl()))
	segChains = append(segChains, head)

	for i, item := range resp.Results {
		if item.Score < stb.LOW_SIMILARITY_THRESHOLD {
			// 匹配度必定按从大到小排序
			if i > 3 {
				// 保留前三条低匹配度结果, 剩余的不展示
				break
			}
		}

		seg, ok := item.Primary()
		if !ok {
			continue
		}

		segChain := make(message.SegmentArray, 0, 4)
		if seg.ThumbnailUrl != "" {
			segChain.Append(message.Image(seg.ThumbnailUrl))
		}
		segChain.Append(message.Textf("%s\n匹配度: %.2f%% | 语言: %s",
			seg.DisplayTitle(), item.Score, seg.Language.Emoji()))

		if seg.SourceUrl != "" {
			segChain.Append(message.Textf("\n详情页：%s", seg.SourceUrl))
		}
		if seg.PageUrl != "" {
			segChain.Append(message.Textf("\n图片页：%s", seg.PageUrl))
		}

		segChains = append(segChains, segChain)
	}

	if len(segChains)-1 < len(resp.Results) {
		segChains = append(segChains, message.SegmentArray{
			message.Textf("未展示剩余 %d 条低匹配度结果", len(resp.Results)-(len(segChains)-1)),
		})
	}

	return segChains
}

// 等比缩放最长边到 longest, 短边按比例缩放
func scaleDownImageJpeg(imgData []byte, longest uint) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, err
	}
	var newImage image.Image
	origWidth, origHeight := img.Bounds().Dx(), img.Bounds().Dy()

	if origWidth >= origHeight {
		if origWidth > int(longest) {
			newImage = resize.Resize(longest, 0, img, resize.Lanczos3)
		} else {
			newImage = img
		}
	} else {
		if origHeight > int(longest) {
			newImage = resize.Resize(0, longest, img, resize.Lanczos3)
		} else {
			newImage = img
		}
	}

	return encodeJpeg(newImage)
}
