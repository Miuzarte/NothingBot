package main

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/tidwall/gjson"
)

// 本文件的日志 scope
var logTiebaParse = logger.New("TiebaParse")

const TIEBA_URL_REGEXP = `tieba\.baidu\.com/p/(\d+)`

var tiebaUrlReg = regexp.MustCompile(TIEBA_URL_REGEXP)

const tiebaParseMId ModuleId = "TiebaParse"

var moduleTiebaParse = Module{
	ModuleMeta: ModuleMeta{
		Name: tiebaParseMId,
		Desc: "百度贴吧链接解析",
	},
	Disable: env.Testing,
}

func initTP() {
	NoBuildPrintFile("M_TiebaParse.go")

	moduleTiebaParse.Init = initTiebaParse
	modules.Add(&moduleTiebaParse)
}

func initTiebaParse() {
	onebot.AddMatcher(moduleTiebaParse.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnRegexpFindAllStringSubmatch(tiebaUrlReg).
		Do(moduleTiebaParse.RWMuWrap(ctxTiebaParse)),
	)
}

func ctxTiebaParse(ctx *EasyOnebot.Ctx) {
	tid := ctx.Submatches.Get(moduleTiebaParse.Name.String())[0][1]
	tp, err := getTiebaPost(tid)
	if err != nil {
		ctx.SendMsgf("获取帖子失败：%v", err)
		return
	}
	reply, err := tp.Format(ctx.Event.Sender.UserId, ctx.Event.Sender.GetCardOrNickname())
	if err != nil {
		ctx.SendMsgf("格式化帖子失败：%v", err)
		return
	}
	_, err = ctx.SendForwardMsgAuto(reply)
	if err != nil {
		ctx.SendMsg("合并转发发送失败")
		return
	}
}

func getTiebaPost(tid string) (*TiebaPost, error) {
	resp, err := http.Get(TIEBA_URL_POST_DETAIL + "?tid=" + tid)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("empty body")
	}
	return unmarshalTiebaPost(string(b))
}

const (
	TIEBA_POST_TYPE_TEXT = iota // 文本
	_
	TIEBA_POST_TYPE_EMO   // 表情
	TIEBA_POST_TYPE_IMAGE // 图片
)

const (
	TIEBA_URL_POST_DETAIL = `https://api.obfs.dev/api/tieba/post_detail`
	TIEBA_URL_PORTRAIT    = `https://himg.bdimg.com/sys/portrait/item/`
	TIEBA_URL_EMO         = `https://gsp0.baidu.com/5aAHeD3nKhI2p27j8IqW0jdnxx1xbK/tb/editor/images/client/` // image_emoticon24.png
)

var bawuTypeMap = map[string]string{
	"manager": "吧主",
	"assist":  "小吧主",
}

type TiebaForum struct {
	Avatar      string
	Name        string
	Content     string
	FirstClass  string
	SecondClass string
}

type TiebaUser struct {
	Id       int64
	Name     string
	NameShow string
	Portrait string // https://himg.bdimg.com/sys/portrait/item/ + Portrait
	LevelId  int64
	IsBawu   bool
	BawuType string
}

type TiebaPost struct {
	PostList []gjson.Result
	Forum    TiebaForum
	UserMap  map[int64]*TiebaUser
}

func unmarshalTiebaPost(s string) (tp *TiebaPost, err error) {
	if len(s) == 0 {
		return nil, errors.New("empty body")
	}
	j := gjson.Parse(s)
	if j.Get("code").Int() != 0 {
		logTiebaParse.Warn().
			Str("body", s).
			Msg("non-zero code")
		return nil, errors.New(s)
	}
	postList := j.Get("post_list").Array()
	if len(postList) == 0 {
		logTiebaParse.Warn().
			Str("body", s).
			Msg("empty post_list")
		return nil, errors.New(s)
	}
	tp = &TiebaPost{
		PostList: postList,
		Forum: TiebaForum{
			Avatar:      j.Get("forum.avatar").String(),
			Name:        j.Get("forum.name").String(),
			Content:     j.Get("forum.show_info.content").String(),
			FirstClass:  j.Get("forum.first_class").String(),
			SecondClass: j.Get("forum.second_class").String(),
		},
		UserMap: make(map[int64]*TiebaUser),
	}
	for _, u := range j.Get("user_list").Array() {
		id := u.Get("id").Int()
		name := u.Get("name").String()
		nameShow := u.Get("name_show").String()
		portrait := u.Get("portrait").String()
		levelId := u.Get("level_id").Int()
		isBawu := u.Get("is_bawu").Bool()
		bawuType := u.Get("bawu_type").String()
		tp.UserMap[id] = &TiebaUser{
			Id:       id,
			Name:     name,
			NameShow: nameShow,
			Portrait: portrait,
			LevelId:  levelId,
			IsBawu:   isBawu,
			BawuType: bawuType,
		}
	}
	return
}

func (tp *TiebaPost) Format(uid int, name string) (forward message.SegmentArray, err error) {
	if len(tp.PostList) == 0 || len(tp.UserMap) == 0 {
		return nil, errors.New("empty post")
	}

	forward = message.SegmentArray{
		message.Node3(uid, name, message.SegmentArray{
			message.Image(tp.Forum.Avatar), // 吧头像
			message.Textf( // 吧名
				"\n%s吧\n%s\n%s - %s",
				tp.Forum.Name,
				tp.Forum.Content,
				tp.Forum.FirstClass,
				tp.Forum.SecondClass,
			),
		}),
	}

	// 贴吧
	for i, post := range tp.PostList {
		segChain := message.SegmentArray{}
		if i == 0 { // 楼主
			segChain.Append(message.Textf("楼主：%s\n\n", post.Get("title").String()))
		}

		user := tp.UserMap[post.Get("author_id").Int()]

		// 头像
		segChain.Append(message.Image(TIEBA_URL_PORTRAIT + user.Portrait))

		// 用户名、吧务、等级
		if user.Name == "" || !strings.HasPrefix(user.NameShow, "贴吧用户_") {
			segChain.Append(message.Textf("\n%s ", user.NameShow))
		} else {
			segChain.Append(message.Textf("\n%s ", user.Name))
		}
		if user.IsBawu {
			bawu := bawuTypeMap[user.BawuType]
			if bawu == "" {
				bawu = user.BawuType
			}
			segChain.Append(message.Textf("[%s]", bawu))
		}
		segChain.Append(message.Textf("(LV.%d)\n\n", user.LevelId))

		// 内容
		content := post.Get("content").Array()
		for _, c := range content {
			switch typ := c.Get("type").Int(); typ {
			case TIEBA_POST_TYPE_TEXT:
				segChain.Append(message.Text(c.Get("text").String()))
			case TIEBA_POST_TYPE_EMO:
				segChain.Append(message.Image(TIEBA_URL_EMO + c.Get("text").String() + ".png"))
			case TIEBA_POST_TYPE_IMAGE:
				segChain.Append(message.Image(c.Get("origin_src").String()))
			default:
				segChain.Append(message.Textf("<未知类型：%d>", typ))
			}
		}

		forward.Append(message.Node3(uid, name, segChain))
	}
	return forward, nil
}
