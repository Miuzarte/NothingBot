package main

import (
	"fmt"
	"strconv"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/robfig/cron/v3"
)

// 本文件的日志 scope
var logCrontab = logger.New("Crontab")

type CrontabConfig struct {
	Crontab string
	Msg     any
	Users   any // []int / int / []string / stirng / []any
	Groups  any
}

type Crontab struct {
	CronID cron.EntryID

	Msg        string
	ForwardMsg message.SegmentArray

	Users  []int
	Groups []int
}

const crontabMId ModuleId = "Crontab"

var (
	crontabs    []*Crontab
	crontabCron = cron.New()
)

var moduleCrontab = Module{
	ModuleMeta: ModuleMeta{
		Name:       crontabMId,
		Desc:       "定时任务",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_Crontab.go")

	moduleCrontab.AfterInit = initCrontab // 构造合并转发需要 uin
	moduleCrontab.ReInit = initCrontab
	modules.Add(&moduleCrontab)
}

func initCrontab() {
	var crontabConfigs []CrontabConfig
	err := config.DecodeModule(crontabMId, &crontabConfigs)
	if err != nil {
		logCrontab.Error().
			Err(err).
			Msg("failed to decode config")
		return
	}

	// stop existing crontabs
	<-crontabCron.Stop().Done()
	for _, c := range crontabs {
		if c.CronID != 0 {
			crontabCron.Remove(c.CronID)
		}
	}

	crontabs = decodeCrontab(crontabConfigs)
	if len(crontabs) == 0 {
		return
	}

	crontabCron.Start()
	for i, ent := range crontabCron.Entries() {
		logCrontab.Debug().
			Int("crontab", i).
			Int("entry", int(ent.ID)).
			Time("next", ent.Next).
			Msg("crontab next")
	}
}

func cronFuncWraper(cron *Crontab) func() {
	return func() {
		if cron.ForwardMsg == nil {
			PushMsg(cron.Msg, cron.Users, cron.Groups)
		} else {
			PushForwardMsg(cron.ForwardMsg, cron.Users, cron.Groups)
		}
	}
}

func decodeCrontab(input []CrontabConfig) (output []*Crontab) {
	var err error
	output = make([]*Crontab, 0, len(input))
	logCrontab.Debug().
		Int("configs", len(input)).
		Msg("found configs")
	for i, config := range input {
		crontab := &Crontab{}

	GROUP:
		if groups := config.Groups; groups != nil {
			switch groups := groups.(type) {
			case []any:
				crontab.Groups = make([]int, 0, len(groups))
				for j, group := range groups {
					switch group := group.(type) {
					case int:
						crontab.Groups = append(crontab.Groups, group)
					case int64:
						crontab.Groups = append(crontab.Groups, int(group))
					case string:
						groupI, err := strconv.Atoi(group)
						if err == nil {
							crontab.Groups = append(crontab.Groups, groupI)
						} else {
							logCrontab.Warn().
								Err(err).
								Int("index", i).
								Int("group", j).
								Msg("invalid crontab config")
						}
					default:
						logCrontab.Warn().
							Str("type", fmt.Sprintf("%T", group)).
							Int("index", i).
							Int("group", j).
							Msg("invalid crontab config")
					}
				}

			case int:
				config.Groups = []any{groups}
				goto GROUP
			case int64:
				config.Groups = []any{groups}
				goto GROUP
			case string:
				config.Groups = []any{groups}
				goto GROUP

			default:
				logCrontab.Warn().
					Str("type", fmt.Sprintf("%T", groups)).
					Int("index", i).
					Msg("invalid crontab config")
			}
		}

	USER:
		if users := config.Users; users != nil {
			switch users := users.(type) {
			case []any:
				crontab.Groups = make([]int, 0, len(users))
				for j, user := range users {
					switch user := user.(type) {
					case int:
						crontab.Users = append(crontab.Users, user)
					case int64:
						crontab.Users = append(crontab.Users, int(user))
					case string:
						userI, err := strconv.Atoi(user)
						if err == nil {
							crontab.Users = append(crontab.Users, userI)
						} else {
							logCrontab.Warn().
								Err(err).
								Int("index", i).
								Int("user", j).
								Msg("invalid crontab config")
						}
					default:
						logCrontab.Warn().
							Str("type", fmt.Sprintf("%T", user)).
							Int("index", i).
							Int("user", j).
							Msg("invalid crontab config")
					}
				}

			case int:
				config.Users = []any{users}
				goto USER
			case int64:
				config.Users = []any{users}
				goto USER
			case string:
				config.Users = []any{users}
				goto USER

			default:
				logCrontab.Warn().
					Str("type", fmt.Sprintf("%T", users)).
					Int("index", i).
					Msg("invalid crontab config")
			}
		}

		switch msg := config.Msg.(type) {
		case string:
			crontab.Msg = msg
		case []any:
			crontab.ForwardMsg = parseForwardReply(msg)
		}

		crontab.CronID, err = crontabCron.AddFunc(config.Crontab, cronFuncWraper(crontab))
		if err != nil {
			logCrontab.Warn().
				Err(err).
				Int("index", i).
				Msg("invalid crontab config")
			continue
		}

		output = append(output, crontab)
	}
	return output
}
