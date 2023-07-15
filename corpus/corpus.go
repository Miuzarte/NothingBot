package corpus

import (
	"example/corpus/datas"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	zero "github.com/wdvxdr1123/ZeroBot"
	//"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	engine     = zero.New()
	v          = viper.New()
	configPath = "./corpus/config.yaml"
)

func initConfig() { //初始化配置文件
	_, err := os.Stat(configPath)
	if err != nil {
		fmt.Println("生成默认语料库配置文件")
		os.WriteFile(configPath, datas.Data, 0644)
	}
}

func initOnRegex() { //注册用户语料库
	logrus.Infof("[corpus] 语料库找到 %d 条", len(v.GetStringSlice("corpus")))
	counts := 0
	for i := range v.GetStringSlice("corpus") {
		k := i
		counts++
		duration := float64(v.GetFloat64(fmt.Sprintf("corpus.%d.delay", k)) * 1000) //yaml里写小数
		logrus.Infof("[corpus] Type of corpus.%d.reply: %T", k, v.Get(fmt.Sprintf("corpus.%d.reply", k)))
		logrus.Infof("[corpus] Count of corpus.%d.reply: %d", k, len(v.GetStringSlice(fmt.Sprintf("corpus.%d.reply", k))))
		scene := func(ctx *zero.Ctx) bool {
			switch v.GetString(fmt.Sprintf("corpus.%d.scene", k)) {
			case "a", "all": // 全部
				return true
			case "g", "group": // 群
				return v.GetString(fmt.Sprintf("corpus.%d.scene", k)) == "g" && ctx.Event.DetailType == "group"
			case "p", "private": // 私
				return v.GetString(fmt.Sprintf("corpus.%d.scene", k)) == "p" && ctx.Event.DetailType == "private"
			default:
				return false
			}
		}
		engine.OnRegex(v.GetString(fmt.Sprintf("corpus.%d.regexp", k)), scene).Handle(func(ctx *zero.Ctx) {
			go func(k int) {
				time.Sleep(time.Millisecond * time.Duration(duration))
				if ctx.Event.GroupID == 0 {
					switch v.Get(fmt.Sprintf("corpus.%d.reply", k)).(type) {
					case string:
						ctx.SendPrivateMessage(
							ctx.Event.UserID, v.GetString(fmt.Sprintf("corpus.%d.reply", k)))
					case []string, []any:
						ctx.SendPrivateMessage(
							ctx.Event.UserID, v.GetString(fmt.Sprintf("corpus.%d.reply.%d", k, rand.Intn(
								len(v.GetStringSlice(fmt.Sprintf("corpus.%d.reply", k)))))))
					}
				} else {
					switch v.Get(fmt.Sprintf("corpus.%d.reply", k)).(type) {
					case string:
						ctx.SendGroupMessage(
							ctx.Event.GroupID, v.GetString(fmt.Sprintf("corpus.%d.reply", k)))
					case []string, []any:
						ctx.SendGroupMessage(
							ctx.Event.GroupID, v.GetString(fmt.Sprintf("corpus.%d.reply.%d", k, rand.Intn(
								len(v.GetStringSlice(fmt.Sprintf("corpus.%d.reply", k)))))))
					}
				}
			}(k)
		})
		println(len(v.GetStringSlice(fmt.Sprintf("corpus.%d.reply", k))))
	}
	logrus.Infof("[corpus] 语料库注册 %d 条", counts)
}

func init() {
	initConfig()
	logrus.Infoln("[corpus] 读取语料库配置")
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetConfigFile(configPath)
	v.ReadInConfig()
	v.WatchConfig()

	initOnRegex()
	v.OnConfigChange(func(in fsnotify.Event) {
		engine.Delete()
		initOnRegex()
	})
}
