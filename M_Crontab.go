package main

import (
	"strconv"
	"time"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot/message"

	"github.com/robfig/cron/v3"
)

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
		log.Error(err)
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
		log.Debugf("[Crontab] crontab[%d](%d) next: %s", i, ent.ID, ent.Next.Format(time.DateTime))
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
	log.Debug("[Crontab] found configs: ", len(input))
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
							log.Warnf("[Crontab] invalid crontab config [%d] groups [%d]: %v", i, j, err)
						}
					default:
						log.Warnf("[Crontab] invalid crontab config [%d] groups [%d]: %T", i, j, group)
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
				log.Warnf("[Crontab] invalid crontab config [%d] groups: %T", i, groups)
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
							log.Warnf("[Crontab] invalid crontab config [%d] users [%d]: %v", i, j, err)
						}
					default:
						log.Warnf("[Crontab] invalid crontab config [%d] users [%d]: %T", i, j, user)
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
				log.Warnf("[Crontab] invalid crontab config [%d] users: %T", i, users)
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
			log.Warnf("[Crontab] invalid crontab config [%d] crontab: %v", i, err)
			continue
		}

		output = append(output, crontab)
	}
	return output
}
