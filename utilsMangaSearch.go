package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"time"

	"NothingBot_v4/logger"
	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/message"
	"golang.org/x/image/webp"

	eh "github.com/Miuzarte/EHentai-go"
	jm "github.com/Miuzarte/JMComic-go"
	nh "github.com/Miuzarte/NHentai-go"
	nhApi "github.com/Miuzarte/NHentai-go/api"
	pc "github.com/Miuzarte/PicaComic-go"
)

// 本文件的日志 scope
var logMangaSearch = logger.New("MangaSearch")

type MangaSearchSite int

const (
	_ MangaSearchSite = iota
	MANGA_SITE_EH
	MANGA_SITE_EXH
	MANGA_SITE_NH
	MANGA_SITE_JM
	MANGA_SITE_PC
)

func (mss MangaSearchSite) String() string {
	switch mss {
	case MANGA_SITE_EH:
		return "EHentai"
	case MANGA_SITE_EXH:
		return "ExHentai"
	case MANGA_SITE_NH:
		return "NHentai"
	case MANGA_SITE_JM:
		return "JMComic"
	case MANGA_SITE_PC:
		return "PicaComic"
	default:
		panic(fmt.Errorf("undefined: %d", mss))
	}
}

const (
	REQUEST_TIMEOUT          = 5 * time.Minute
	RECALL_DURATION          = 100 * time.Second
	MANGA_SEARCH_MAX_RESULTS = 8
)

type MangaSearch struct {
	Ctx     context.Context
	Site    MangaSearchSite
	Keyword string
	All     bool

	NodeUserId   int
	NodeNickname string
}

func NewMangaSearch(ctx *EasyOnebot.Ctx, tctx context.Context, site MangaSearchSite, keyword string) MangaSearch {
	return MangaSearch{
		Ctx:     tctx,
		Site:    site,
		Keyword: keyword,

		NodeUserId:   ctx.Event.Sender.UserId,
		NodeNickname: ctx.Event.Sender.GetCardOrNickname(),
	}
}

func (ms *MangaSearch) Do() (forward message.SegmentArray, err error) {
	const (
		defaultEhSearchCategory = //
		eh.CATEGORY_DOUJINSHI |
			eh.CATEGORY_MANGA |
			eh.CATEGORY_NON_H |
			eh.CATEGORY_COSPLAY
		defaultJmSearchOrder = "mr"
	)

	headerSegChain := message.SegmentArray{
		message.Textf("一分钟后撤回，提前转发走\n%s搜索：%s", ms.Site, ms.Keyword),
	}
	var resultsSegChains []message.SegmentArray
	switch ms.Site {
	case MANGA_SITE_EH, MANGA_SITE_EXH:
		cat := defaultEhSearchCategory
		if ms.All {
			cat = eh.CATEGORY_ALL
		}
		var total int
		var results eh.FSearchResults
		var err error
		switch ms.Site {
		case MANGA_SITE_EH:
			total, results, err = eh.FSearch(ms.Ctx, eh.EHENTAI_URL, ms.Keyword, cat)
		case MANGA_SITE_EXH:
			total, results, err = eh.FSearch(ms.Ctx, eh.EXHENTAI_URL, ms.Keyword, cat)
		}
		if err != nil {
			return nil, err
		}

		if len(results) == 0 {
			return nil, fmt.Errorf("no result")
		}

		headerSegChain.Append(ms.buildHeaderEh(total, &results)...)
		switch ms.Site {
		case MANGA_SITE_EH:
			resultsSegChains = ms.buildResultsEh(results)
		case MANGA_SITE_EXH:
			resultsSegChains = ms.buildResultsEh(results)
			// resultsSegChains = ms.buildResultsEhNoCover(results)
		}

	case MANGA_SITE_NH:
		search, err := nh.Search(ms.Ctx, ms.Keyword, 0, "")
		if err != nil {
			return nil, err
		}

		if len(search.Result) == 0 {
			return nil, fmt.Errorf("no result")
		}

		headerSegChain.Append(ms.buildHeaderNh(search)...)
		resultsSegChains = ms.buildResultsNh(search)

	case MANGA_SITE_JM:
		search, err := jm.Search(ms.Ctx, ms.Keyword, defaultJmSearchOrder, 0)
		if err != nil {
			return nil, err
		}

		if len(search.Content) == 0 {
			return nil, fmt.Errorf("no result")
		}

		headerSegChain.Append(ms.buildHeaderJm(search)...)
		resultsSegChains = ms.buildResultsJm(search)

	case MANGA_SITE_PC:
		search, err := pc.Search(ms.Ctx, ms.Keyword, nil, "", 0)
		if err != nil {
			return nil, err
		}

		if len(search.Comics.Docs) == 0 {
			return nil, fmt.Errorf("no result")
		}

		headerSegChain.Append(ms.buildHeaderPc(search)...)
		resultsSegChains = ms.buildResultsPc(search)

	default:
		panic(fmt.Errorf("unexpected manga site: %d", ms.Site))
	}

	forward = make(message.SegmentArray, 0, 1+len(resultsSegChains))
	forward.Append(message.Node3(ms.NodeUserId, ms.NodeNickname, headerSegChain))
	for _, segChain := range resultsSegChains {
		forward.Append(message.Node3(ms.NodeUserId, ms.NodeNickname, segChain))
	}

	return forward, nil
}

func (ms *MangaSearch) buildHeaderEh(total int, results *eh.FSearchResults) message.SegmentArray {
	segChain := make(message.SegmentArray, 0, 2)
	segChain.Append(message.Textf("\n共%d个结果", total))
	if len(*results) > MANGA_SEARCH_MAX_RESULTS {
		*results = (*results)[:MANGA_SEARCH_MAX_RESULTS]
		segChain.Append(message.Textf("，展示前%d个", len(*results)))
	}
	return segChain
}

func (ms *MangaSearch) buildResultsEh(results eh.FSearchResults) []message.SegmentArray {
	segChains := make([]message.SegmentArray, 0, len(results))
	i := 0
	for cover, err := range eh.DownloadCoversIter(ms.Ctx, results) {
		result := &results[i]
		segChain := make(message.SegmentArray, 0, 2)
		if err != nil {
			logMangaSearch.Warn().
				Err(err).
				Int("index", i).
				Msg("failed to download search results cover")
			segChain.Append(message.Text("<COVER DOWNLOAD FAILED>\n"))
		} else if ehRatingCmp(result.Rating, mangaConfig.EHentaiCoverShowRating) > 0 {
			segChain.Append(message.Image(salt(cover.Data, mangaConfig.SaltLength)))
		} else {
			segChain.Append(message.Text("<COVER HIDED>\n"))
		}
		segChain.Append(message.Text(ehFormatGalleryInfo(result)))

		segChains = append(segChains, segChain)
		i++
	}
	return segChains
}

func (ms *MangaSearch) buildResultsEhNoCover(results eh.FSearchResults) []message.SegmentArray {
	segChains := make([]message.SegmentArray, 0, len(results))
	for i := range results {
		segChain := message.SegmentArray{
			message.Text("<COVER HIDED>\n"),
			message.Text(ehFormatGalleryInfo(&results[i])),
		}
		segChains = append(segChains, segChain)
	}
	return segChains
}

func (ms *MangaSearch) buildHeaderNh(search *nhApi.PaginatedResponseGalleryListItem) message.SegmentArray {
	segChain := make(message.SegmentArray, 0, 2)
	perPage := 0
	if search.PerPage != nil {
		perPage = *search.PerPage
	}
	if search.NumPages > 1 && perPage > 0 {
		segChain.Append(message.Textf("\n共约%d个结果", search.NumPages*perPage))
	} else {
		segChain.Append(message.Textf("\n共%d个结果", len(search.Result)))
	}
	if len(search.Result) > MANGA_SEARCH_MAX_RESULTS {
		search.Result = search.Result[:MANGA_SEARCH_MAX_RESULTS]
		segChain.Append(message.Textf("，展示前%d个", len(search.Result)))
	}
	return segChain
}

func (ms *MangaSearch) buildResultsNh(search *nhApi.PaginatedResponseGalleryListItem) []message.SegmentArray {
	results := nhApi.GalleryListItems(search.Result)
	segChains := make([]message.SegmentArray, 0, len(results))
	i := 0
	for cover, err := range results.DownloadCoversIter(ms.Ctx) {
		result := results[i]
		segChain := make(message.SegmentArray, 0, 2)
		if err != nil {
			logMangaSearch.Warn().
				Err(err).
				Int("index", i).
				Msg("failed to download search results cover")
			segChain.Append(message.Text("<COVER DOWNLOAD FAILED>\n"))
		} else {
			segChain.Append(message.Image(salt(cover.Data, mangaConfig.SaltLength)))
		}
		tagIds := []int{}
		if result.TagIds != nil {
			tagIds = *result.TagIds
		}
		tags, _ := nh.LookupTags(ms.Ctx, tagIds...)
		segChain.Append(message.Text(nhFormatGalleryInfo(&result, tags)))

		segChains = append(segChains, segChain)
		i++
	}
	return segChains
}

func (ms *MangaSearch) buildHeaderJm(search *jm.SearchResp) message.SegmentArray {
	segChain := make(message.SegmentArray, 0, 2)
	segChain.Append(message.Textf("\n共%d个结果", search.Total))
	if len(search.Content) > MANGA_SEARCH_MAX_RESULTS {
		search.Content = search.Content[:MANGA_SEARCH_MAX_RESULTS]
		segChain.Append(message.Textf("，展示前%d个", len(search.Content)))
	}
	return segChain
}

func (ms *MangaSearch) buildResultsJm(search *jm.SearchResp) []message.SegmentArray {
	segChains := make([]message.SegmentArray, 0, len(search.Content))
	i := 0
	for cover, err := range jm.DownloadCoversIter(ms.Ctx, search) {
		segChain := make(message.SegmentArray, 0, 2)
		if err != nil {
			logMangaSearch.Warn().
				Err(err).
				Int("index", i).
				Msg("failed to download search results cover")
			segChain.Append(message.Text("<COVER DOWNLOAD FAILED>\n"))
		} else {
			segChain.Append(message.Image(salt(cover.Data, mangaConfig.SaltLength)))
		}
		segChain.Append(message.Text(jmFormatGalleryInfo(search.Content[i])))

		segChains = append(segChains, segChain)
		i++
	}
	return segChains
}

func (ms *MangaSearch) buildHeaderPc(search *pc.SearchResp) message.SegmentArray {
	segChain := make(message.SegmentArray, 0, 2)
	segChain.Append(message.Textf("\n共%d个结果", search.Comics.Total))
	if len(search.Comics.Docs) > MANGA_SEARCH_MAX_RESULTS {
		search.Comics.Docs = search.Comics.Docs[:MANGA_SEARCH_MAX_RESULTS]
		segChain.Append(message.Textf("，展示前%d个", len(search.Comics.Docs)))
	}
	return segChain
}

func (ms *MangaSearch) buildResultsPc(search *pc.SearchResp) []message.SegmentArray {
	segChains := make([]message.SegmentArray, 0, len(search.Comics.Docs))
	i := 0
	for cover, err := range pc.DownloadCoversIter(ms.Ctx, search) {
		segChain := make(message.SegmentArray, 0, 2)
		if err != nil {
			logMangaSearch.Warn().
				Err(err).
				Int("index", i).
				Msg("failed to download search results cover")
			segChain.Append(message.Text("<COVER DOWNLOAD FAILED>\n"))
		} else {
			segChain.Append(message.Image(salt(cover.Data, mangaConfig.SaltLength)))
		}
		segChain.Append(message.Text(pcFormatGalleryInfo(&search.Comics.Docs[i])))

		segChains = append(segChains, segChain)
		i++
	}
	return segChains
}

func ehRatingCmp(rating1, rating2 any) int {
	var r1, r2 float64
	switch v := rating1.(type) {
	case float64:
		r1 = v
	case float32:
		r1 = float64(v)
	case string:
		r1, _ = strconv.ParseFloat(v, 64)
	}
	switch v := rating2.(type) {
	case float64:
		r2 = v
	case float32:
		r2 = float64(v)
	case string:
		r2, _ = strconv.ParseFloat(v, 64)
	}
	return cmp.Compare(r1, r2)
}

func ehFormatGalleryInfo(result *eh.FSearchResult) string {
	gUrl := result.Url
	gUrl = strings.TrimPrefix(gUrl, "https://")
	gUrl = strings.TrimSuffix(gUrl, "/")
	return fmt.Sprintf(
		`%s
%s
%s | %.2f⭐ | %dP
%s`,
		result.Title,
		strings.Join(eh.TranslateMulti(result.Tags), "，"),
		result.Cat, result.Rating, result.Pages,
		gUrl,
	)
}

// [TODO] tags adapter
func translateNhTags(tags nhApi.Tags) string {
	tagSb := strings.Builder{}
	tagStrings := tags.Strings()
	ehTags, err := eh.ParseTags(tagStrings)
	if err == nil {
		tagSets := eh.TranslateTags(ehTags).Set()
		for i, set := range tagSets {
			tagSb.WriteString(set.Namespace)
			tagSb.WriteString("：")
			tagSb.WriteString(strings.Join(set.Tags, "，"))
			if i != len(tagSets)-1 {
				tagSb.WriteString("\n")
			}
		}
	} else {
		tagSb.WriteString(strings.Join(eh.TranslateMulti(tagStrings), "，"))
	}
	return tagSb.String()
}

func nhFormatGalleryInfo(g *nhApi.GalleryListItem, tags nhApi.Tags) string {
	gUrl := fmt.Sprintf("nhentai.net/g/%d", g.Id)
	japaneseTitle := ""
	if g.JapaneseTitle != nil {
		japaneseTitle = *g.JapaneseTitle
	}
	numPages := 0
	if g.NumPages != nil {
		numPages = *g.NumPages
	}
	numFavorites := 0
	if g.NumFavorites != nil {
		numFavorites = *g.NumFavorites
	}
	return fmt.Sprintf(
		`%s
%s
%d❤️ | %dP
%s`,
		japaneseTitle,
		translateNhTags(tags),
		numFavorites, numPages,
		gUrl,
	)
}

func jmFormatGalleryInfo(content jm.ComicBasic) string {
	return fmt.Sprintf(
		`%s
%s
18comic.vip/album/%s`,
		content.Name,
		content.Author,
		content.Id,
	)
}

func pcFormatGalleryInfo(comic *pc.SearchComic) string {
	var comicFin string
	if comic.Finished {
		comicFin = " (完)"
	}
	return fmt.Sprintf(
		`%s%s
%s
%d❤️
pica://%s`,
		comic.Title, comicFin,
		strings.Join(comic.Categories, "，"),
		comic.LikesCount,
		comic.Id,
	)
}

func registerTimerRecall(msgId int64) {
	go func() {
		<-time.After(RECALL_DURATION)
		onebot.Call().Std.DeleteMsg(int(msgId))
	}()
}

func imgToJpg(data []byte) (jpg []byte, bounds image.Rectangle, err error) {
	var img image.Image
	ct := http.DetectContentType(data)
	switch ct {
	case "image/webp":
		img, err = webp.Decode(bytes.NewReader(data))
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
		if img != nil {
			return data, img.Bounds(), err
		}
	case "image/png":
		img, err = png.Decode(bytes.NewReader(data))
	default:
		return nil, bounds, fmt.Errorf("unsupported type: %s", ct)
	}
	if err != nil {
		return nil, bounds, fmt.Errorf("failed to decode image: %w", err)
	}
	jpg, err = encodeJpeg(img)
	if err != nil {
		return nil, bounds, fmt.Errorf("failed to encode image: %w", err)
	}
	return jpg, img.Bounds(), nil
}

var jpgOp = jpeg.Options{Quality: 95}

func encodeJpeg(img image.Image) (jpg []byte, err error) {
	buf := bytes.Buffer{}
	err = jpeg.Encode(&buf, img, &jpgOp)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func salt(imgData []byte, length int) []byte {
	salted := make([]byte, len(imgData)+length)
	copy(salted, imgData)
	rand.Read(salted[len(imgData):])
	return salted
}
