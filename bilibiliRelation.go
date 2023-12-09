package main

import (
	"encoding/json"
	"fmt"
	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
	"strconv"
	"time"
)

type relationStat struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Ttl     int    `json:"ttl"`
	Data    struct {
		Mid       int `json:"mid"`
		Following int `json:"following"`
		Whisper   int `json:"whisper"`
		Black     int `json:"black"`
		Follower  int `json:"follower"`
	} `json:"data"`
}

func initListen() {
	listLen := len(v.GetStringSlice("listen.list"))
	for i := 0; i < listLen; i++ {
		uid := v.GetInt(fmt.Sprint("listen.list.", i, ".uid"))
		target := v.GetInt(fmt.Sprint("listen.list.", i, ".target"))

		groupsRaw := v.GetStringSlice(fmt.Sprint("listen.list.", i, ".group"))
		groups := []int{}
		for i2, groupRaw := range groupsRaw {
			g, err := strconv.Atoi(groupRaw)
			if err != nil {
				log.Error("[Relation] listen.list.", i, ".group.", i2, " 格式错误: ", groupRaw)
				continue
			}
			groups = append(groups, g)
		}
		usersRaw := v.GetStringSlice(fmt.Sprint("listen.list.", i, ".user"))
		users := []int{}
		for i2, userRaw := range usersRaw {
			g, err := strconv.Atoi(userRaw)
			if err != nil {
				log.Error("[Relation] listen.list.", i, ".group.", i2, " 格式错误: ", userRaw)
				continue
			}
			users = append(users, g)
		}
		go listenFollowers(uid, target, groups, users)
	}
}

func listenFollowers(uid any, target int, groups, users []int) {
	log.Debug("[Relation] listen followers of ", uid, " til ", target, " for ", groups)
	i := ihttp.New().WithUrl("https://api.bilibili.com/x/relation/stat?vmid=1954091502").
		WithAddQuerys(map[string]any{"vmid": uid}).
		WithHeaders(iheaders).WithCookie(biliIdentity.Cookie)
	lastStat := &relationStat{}
	for {
		resp, err := i.Get().ToBytes()
		if err != nil {
			log.Error("[Relation] stat of ", uid, " 请求失败: ", err)
			time.Sleep(time.Second * 10)
			continue
		}
		stat := &relationStat{}
		err = json.Unmarshal(resp, stat)
		if err != nil {
			log.Error(
				"[Relation] stat of ", uid, " 反序列化出错: ", err,
				"\n    resp: ", BytesToString(resp),
				"\n    Unmarshal by gson: ", gson.New(resp).JSON("", ""),
			)
			time.Sleep(time.Second * 10)
			continue
		}
		if stat.Code != 0 {
			log.Error("[Relation] stat of ", uid, " 响应出错: ", *stat)
			time.Sleep(time.Second * 10)
			continue
		}

		last, now := lastStat.Data.Follower, stat.Data.Follower
		if last != now && last != 0 {
			change := func() string {
				if change := now - last; change > 0 {
					return fmt.Sprint("  (+", change, ")")
				} else if change < 0 {
					return fmt.Sprint("  (", change, ")")
				}
				return ""
			}()
			msg := fmt.Sprint("[Relation] ", uid, " 粉丝数: ", last, " -> ", now, change)
			log.Info(msg)
		}
		if stat.Data.Follower >= target {
			msg := fmt.Sprint(
				"[Relation] uid ", uid,
				" 在 ", time.Now().Format("2006/01/02-15/04/05"),
				" 达到 ", target, " 粉丝数",
			)
			log.Info(msg)
			bot.Log2SU.Info(msg)
			bot.SendPrivateMsgs(users, msg)
			bot.SendGroupMsgs(groups, msg)
			return
		}
		lastStat = stat
		time.Sleep(time.Second)
	}
}
