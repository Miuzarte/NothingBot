package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/biligo"
)

// 本文件的日志 scope
var logBiliRelation = logger.New("BiliRelation")

type BiliRelationConfig struct {
	Uid    int // 主要判断
	Target int
	Groups []int

	uid    string
	name   string
	ctx    context.Context
	cancel context.CancelFunc
}

type BiliRelationConfigs []*BiliRelationConfig // uid: config

const biliRelationMId ModuleId = "BiliRelation"

var biliRelationConfigs BiliRelationConfigs

var moduleBiliRelation = Module{
	ModuleMeta: ModuleMeta{
		Name:       biliRelationMId,
		Desc:       "Bilibili粉丝数监控",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_BiliRelation.go")

	moduleBiliRelation.AfterInit = initBiliRelation
	moduleBiliRelation.ReInit = initBiliRelation
	moduleBiliRelation.AtExit = biliRelationConfigs.StopAll
	modules.Add(&moduleBiliRelation)
}

func initBiliRelation() {
	newConfigs := BiliRelationConfigs{}
	err := config.DecodeModule(biliRelationMId, &newConfigs)
	if err != nil {
		logBiliRelation.Error().
			Err(err).
			Msg("failed to decode config")
		return
	}
	if len(newConfigs) == 0 {
		if len(biliRelationConfigs) != 0 {
			biliRelationConfigs.StopAll()
			biliRelationConfigs = nil
		}
		return
	}
	if len(biliRelationConfigs) == 0 {
		biliRelationConfigs = newConfigs
		go biliRelationConfigs.RunAll()
		return
	}

	// 更新已有的
	for _, config := range biliRelationConfigs {
		if newConfig := newConfigs.Get(config.Uid); newConfig != nil {
			config.Target = newConfig.Target
			config.Groups = newConfig.Groups
		}
	}
	// 找出新增的
	addedConfigs := make(BiliRelationConfigs, 0)
	for _, config := range newConfigs {
		if biliRelationConfigs.Get(config.Uid) == nil {
			addedConfigs.Add(config)
		}
	}
	// 找出删除的
	deletedConfigs := make(BiliRelationConfigs, 0)
	for _, config := range biliRelationConfigs {
		if newConfigs.Get(config.Uid) == nil {
			deletedConfigs.Add(config)
		}
	}
	// 更新配置
	biliRelationConfigs.Add(addedConfigs...)
	biliRelationConfigs.Remove(deletedConfigs...)
	// 运行/停止
	go addedConfigs.RunAll()
	go deletedConfigs.StopAll()
}

func (brc *BiliRelationConfigs) RunAll() {
	for _, config := range *brc {
		if config.cancel != nil {
			config.cancel()
		}
		config.uid = Itoa(config.Uid)
		j, err := biligo.FetchSpaceCard(config.uid)
		if err != nil {
			msg := fmt.Sprintf("[BiliRelation] %s failed to fetch name: %v", config.uid, err)
			logBiliRelation.Error().Msg(msg)
			continue
		}
		config.name = j.Card.Name

		config.ctx, config.cancel = context.WithCancel(context.Background())
		go func(config *BiliRelationConfig) {
			var rs biligo.RelationStat
			var err error
			var duration time.Duration

		mainLoop:
			for {
				rs, err = biligo.FetchRelationStat(config.uid)
				if err != nil {
					logBiliRelation.Error().
						Err(err).
						Str("name", config.name).
						Str("uid", config.uid).
						Msg("failed to fetch relation stat")
					goto FAILED
				}

				go func() {
					err := relationAppendToCsv(config.name, config.uid, time.Now(), rs.Follower)
					if err != nil {
						logBiliRelation.Error().
							Err(err).
							Msg("failed to append to csv")
					}
				}()

				if rs.Follower >= config.Target {
					logBiliRelation.Info().
						Str("uid", config.uid).
						Int("follower", rs.Follower).
						Msg("target reached")
					t := time.Now().Format("2006-01-02 15:04:05")
					msg := fmt.Sprintf("[BiliRelation] %s(%s) 在 %s 达成 %d 粉丝数", config.name, config.uid, t, rs.Follower)
					logBiliRelation.Info().Msg(msg)
					PushMsg(msg, nil, config.Groups)
					config.cancel()
					break mainLoop
				}

				duration = time.Second * time.Duration(config.Target-rs.Follower)
				logBiliRelation.Info().
					Str("name", config.name).
					Str("uid", config.uid).
					Int("follower", rs.Follower).
					Int("target", config.Target).
					Int("diff", rs.Follower-config.Target).
					Dur("next", duration).
					Msg("bili relation progress")

				select {
				case <-time.After(duration):
					continue mainLoop
				case <-config.ctx.Done():
					break mainLoop
				}

			FAILED:
				<-time.After(time.Second * 10)
				continue mainLoop
			}
		}(config)
	}
}

func (brc *BiliRelationConfigs) StopAll() {
	for _, config := range *brc {
		if config.cancel != nil {
			config.cancel()
			config.cancel = nil
		}
	}
}

func (brc *BiliRelationConfigs) Add(configs ...*BiliRelationConfig) {
	*brc = append(*brc, configs...)
}

func (brc *BiliRelationConfigs) Remove(configs ...*BiliRelationConfig) {
	for _, config := range configs {
		for i, c := range *brc {
			if c.Uid == config.Uid {
				*brc = slices.Delete((*brc), i, i+1)
				break
			}
		}
	}
}

func (brc *BiliRelationConfigs) Get(uid int) *BiliRelationConfig {
	for _, config := range *brc {
		if config.Uid == uid {
			return config
		}
	}
	return nil
}

const RELATION_FILE_PATH = `bili_relation.csv`

func relationAppendToCsv(name, uid string, t time.Time, follower int) error {
	file, err := os.OpenFile(RELATION_FILE_PATH, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "%s,%s,%s,%d\n", name, uid, t.Format("2006-01-02 15:04:05"), follower)
	return err
}
