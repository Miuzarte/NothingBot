package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"
	"github.com/Miuzarte/EasyOnebot/message"
)

// 本文件的日志 scope
var logCorpus = logger.New("Corpus")

const (
	CORPUS_SCENE_ALL     byte = 'a'
	CORPUS_SCENE_GROUP   byte = 'g'
	CORPUS_SCENE_PRIVATE byte = 'p'
)

type CorpusConfig struct {
	Regexp string
	Reply  any
	Scene  string
	Delay  string
}

type Corpus struct {
	Reg *regexp.Regexp

	Reply           string
	ReplyForwardMsg message.SegmentArray

	Scene byte
	Delay time.Duration
}

const corpusMId ModuleId = "Corpus"

var corpuses []Corpus

// [TODO] 重构: 初始化时直接构建[*EasyOnebot.Matcher], 发送的消息放weakptr
// 用ctx.SelfId替代uid: 0
var moduleCorpus = Module{
	ModuleMeta: ModuleMeta{
		Name:       corpusMId,
		Desc:       "语料库",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_Corpus.go")

	moduleCorpus.AfterInit = initCorpus // 构造合并转发需要 uin
	moduleCorpus.ReInit = initCorpus
	modules.Add(&moduleCorpus)
}

func initCorpus() {
	var corpusConfigs []CorpusConfig
	err := config.DecodeModule(corpusMId, &corpusConfigs)
	if err != nil {
		logCorpus.Error().
			Err(err).
			Msg("failed to decode config")
		return
	}

	corpuses = decodeCorpus(corpusConfigs)
	onebot.AddMatcher(moduleCorpus.Name.String(), EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		IsNotCardMsg().
		Do(moduleCorpus.RWMuWrap(ctxCorpus)),
	)
}

func ctxCorpus(ctx *EasyOnebot.Ctx) {
	for _, corpus := range corpuses {
		scene := (corpus.Scene == CORPUS_SCENE_ALL) ||
			(corpus.Scene == CORPUS_SCENE_GROUP && ctx.Event.TypeL2 == "group") ||
			(corpus.Scene == CORPUS_SCENE_PRIVATE && ctx.Event.TypeL2 == "private")
		if !scene {
			continue
		}
		if corpus.Reg.MatchString(ctx.Event.RawMessage) {
			if corpus.Delay > 0 {
				time.Sleep(corpus.Delay)
			}
			if corpus.ReplyForwardMsg == nil {
				ctx.SendMsg(corpus.Reply)
			} else {
				ctx.SendForwardMsgAuto(corpus.ReplyForwardMsg)
			}
		}
	}
}

func decodeCorpus(input []CorpusConfig) (output []Corpus) {
	var err error
	output = make([]Corpus, 0, len(input))
	logCorpus.Debug().
		Int("configs", len(input)).
		Msg("found configs")
	for i, config := range input {
		var corpus Corpus

		switch config.Scene {
		case "a", "all":
			corpus.Scene = CORPUS_SCENE_ALL
		case "g", "group":
			corpus.Scene = CORPUS_SCENE_GROUP
		case "p", "private":
			corpus.Scene = CORPUS_SCENE_PRIVATE
		default:
			corpus.Scene = CORPUS_SCENE_ALL
		}

		if config.Delay != "" {
			corpus.Delay, err = time.ParseDuration(config.Delay)
			if err != nil {
				logCorpus.Warn().
					Err(err).
					Int("index", i).
					Msg("invalid corpus config")
				continue
			}
		}

		switch reply := config.Reply.(type) {
		case string:
			corpus.Reply = reply
		case []any:
			corpus.ReplyForwardMsg = parseForwardReply(reply)
		}

		var reg *regexp.Regexp
		reg, err = regexp.Compile(config.Regexp)
		if err != nil {
			logCorpus.Warn().
				Err(err).
				Int("index", i).
				Msg("invalid corpus config")
			continue
		}
		corpus.Reg = reg

		output = append(output, corpus)
	}
	return output
}

func mapKeysToLower[T any](m map[string]T) map[string]T {
	mm := make(map[string]T)
	for k, v := range m {
		mm[strings.ToLower(k)] = v
	}
	return mm
}

func parseForwardReply(v []any) (reply message.SegmentArray) {
	li, _ := onebot.GetLoginInfoCache()
	uin := Itoa(li.UserId)
	reply = message.SegmentArray{}
	for _, v := range v {
		switch v := v.(type) {
		case string:
			reply.Append(message.Node3(uin, "", v))

		case map[string]any:
			v = mapKeysToLower(v)

			name := "" // 留空时 lagrange 使用 bot 昵称
			switch n := v["name"].(type) {
			case string:
				name = n
			case nil:
			default:
				name = fmt.Sprint(n)
			}

			uin := uin // 为 0 时头像为🐧
			switch u := v["uin"].(type) {
			case string:
				uin = u
			case int:
				uin = Itoa(u)
			case int64:
				uin = Itoa(u)
			case int32:
				uin = Itoa(u)
			case nil:
			default:
				logCorpus.Warn().
					Str("type", fmt.Sprintf("%T", u)).
					Msg("invalid reply uin type")
			}

			switch c := v["content"].(type) {
			case string:
				reply.Append(message.Node3(uin, name, c))
			case []any:
				for _, c := range c {
					switch c := c.(type) {
					case string:
						reply.Append(message.Node3(uin, name, c))
					default:
						reply.Append(message.Node3(uin, name, fmt.Sprint(c)))
					}
				}

			case nil:
				logCorpus.Warn().Msg("reply content is nil")
				continue
			default:
				logCorpus.Warn().
					Str("type", fmt.Sprintf("%T", c)).
					Msg("invalid reply content type")
				continue
			}
		}
	}
	return reply
}
