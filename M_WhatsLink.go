package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"
	"NothingBot_v4/utils"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

// 本文件的日志 scope
var logWhatsLink = logger.New("WhatsLink")

// {"error":"quota_limited","type":"UNKNOWN","file_type":"","name":"您的請求過於頻繁，請聯繫 https://whatslink.info 獲取更高額度","size":0,"count":0,"screenshots":null}

const P2P_URL_REGEXP = `magnet:\?xt=urn:[a-zA-Z0-9]+:[a-zA-Z0-9]{32,40}|ed2k://\|file\|[^\\/:*?"<>|\r\n]+\|\d{1,20}\|[0-9a-fA-F]{32}\|/`

var p2pUrlReg = regexp.MustCompile(P2P_URL_REGEXP)

const whatsLinkMId ModuleId = "WhatsLink"

var moduleWhatsLink = Module{
	ModuleMeta: ModuleMeta{
		Name: whatsLinkMId,
		Desc: "Magnet/Ed2k解析",
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_WhatsLink.go")

	moduleWhatsLink.Init = initWhatsLink
	modules.Add(&moduleWhatsLink)
}

func initWhatsLink() {
	onebot.AddMatcher(moduleWhatsLink.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		OnRegexpFindAllStringSubmatch(p2pUrlReg).
		Do(moduleWhatsLink.RWMuWrap(ctxWhatsLink)),
	)
}

func ctxWhatsLink(ctx *EasyOnebot.Ctx) {
	uid := ctx.Event.Sender.UserId
	name := ctx.Event.Sender.GetCardOrNickname()

	link := ctx.Submatches.Get(moduleWhatsLink.Name.String())[0][0]
	resp, err := whatsLinkGet(link)
	if err != nil {
		logWhatsLink.Warn().
			Err(err).
			Msg("failed to get api")
		return
	}
	if resp.Error != "" {
		logWhatsLink.Warn().
			Str("error", resp.Error).
			Msg("whatslink api returned error")
		return
	}

	forward := message.SegmentArray{
		message.Node3(uid, name, message.Textf(
			"Name: %s\nType: %s\nCount: %d\nSize: %s\n\n%s",
			resp.Name,
			resp.FileType,
			resp.Count,
			utils.FormatBytes(uint64(resp.Size)),
			link,
		)),
	}
	if len(resp.Screenshots) != 0 {
		imgChain := make(message.SegmentArray, 0, len(resp.Screenshots))
		for _, ss := range resp.Screenshots {
			imgChain.Append(message.Image(ss.Screenshot))
		}
		forward.Append(message.Node3(uid, name, imgChain))
	}

	_, err = ctx.SendForwardMsgAuto(forward)
	if err != nil {
		logWhatsLink.Warn().
			Err(err).
			Msg("failed to send forward")
		return
	}
}

const (
	WHATS_LINK_API   = `https://whatslink.info/api/v1/link`
	WHATS_LINK_PARAM = `?url=`
)

func whatsLinkGet(link string) (*whatsLinkResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, WHATS_LINK_API+WHATS_LINK_PARAM+link, nil)
	if err != nil {
		return nil, err
	}
	hResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer hResp.Body.Close()

	resp := &whatsLinkResponse{}
	err = json.NewDecoder(hResp.Body).Decode(resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type whatsLinkResponse struct {
	Error       string `json:"error"`
	Type        string `json:"type"`
	FileType    string `json:"file_type"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Count       int64  `json:"count"`
	Screenshots []struct {
		Time       int64  `json:"time"`
		Screenshot string `json:"screenshot"` // thumbnail url
	} `json:"screenshots"`
}
