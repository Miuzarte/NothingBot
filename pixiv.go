package main

import (
	"NothinBot/EasyBot"
	"fmt"
	"regexp"
	"strconv"

	"github.com/moxcomic/ihttp"
)

var (
	pixivEnable = false
	pUrl        = map[string]string{
		"cat": "https://pixiv.cat/",
		"re":  "https://pixiv.re/",
	}
)

type pixiv struct {
	pid int
	num int
}

func initPixiv() {
	pixivEnable = v.GetBool("pixiv.enable")
}

func checkPixiv(ctx *EasyBot.CQMessage) {
	//开关控制
	matches := ctx.RegexpFindAllStringSubmatch(`(开启|启用|关闭|禁用)pixiv`)
	if len(matches) > 0 && ctx.IsPrivateSU() {
		switch matches[0][1] {
		case "开启", "启用":
			pixivEnable = true
			ctx.SendMsg("pixiv已启用")
		case "关闭", "禁用":
			pixivEnable = false
			ctx.SendMsg("pixiv已禁用")
		}
		return
	}
	if !pixivEnable {
		return
	}
	match := ctx.RegexpFindAllStringSubmatch(`[看康k]{2}([Pp]|[Pp]站|[Pp][Ii][Dd]|[Pp][Ii][Xx][Ii][Vv])([0-9]+)`)
	if len(match) > 0 && ctx.IsToMe() {
		pid, _ := strconv.Atoi(match[0][2])
		p := &pixiv{
			pid: pid,
		}
		p, err := p.getPicNum()
		if err != nil {
			ctx.SendMsgReply("[pixiv] 获取图片数量失败\n", err.Error())
			return
		}
		content := []string{fmt.Sprint("在 pixiv.net/i/", p.pid, " 下共有 ", p.num, " 张图片")}
		ctx.SendForwardMsg(EasyBot.FastNewForwardMsg(
			"NothingBot", ctx.UserID, 0, 0, append(content, p.getPicUrl()...)...))
	}
}

// 获取图片数
func (p *pixiv) getPicNum() (*pixiv, error) {
	html, err := ihttp.New().WithUrl(fmt.Sprint(pUrl["re"], p.pid, "-99.jpg")).Get().ToString()
	if err != nil {
		return p, err
	}
	//This work has *just one* image, please remove page number from URL.
	//This work has *5* pages, please provide a valid page number.
	match := regexp.MustCompile(`This work has (just one|[0-9]+) (image|pages)`).FindAllStringSubmatch(html, -1)
	if len(match) > 0 {
		if match[0][1] == "just one" {
			p.num = 1
		} else {
			p.num, err = strconv.Atoi(match[0][1])
		}
	}
	return p, err
}

// 生成url
func (p *pixiv) getPicUrl() (images []string) {
	if p.num == 1 {
		images = append(images, fmt.Sprint("[CQ:image,file=", pUrl["re"], p.pid, ".jpg]"))
	} else {
		for i := 0; i < p.num; i++ {
			images = append(images, fmt.Sprint("[CQ:image,file=", pUrl["re"], p.pid, "-", i+1, ".jpg]"))
		}
	}
	return
}
