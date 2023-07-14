package corpus

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	engine = zero.New()
	v      = viper.New()
)

func initOnRegex() { //注册用户语料库
	logrus.Infof("[corpus] 语料库找到 %d 条", len(v.GetStringMap("corpus")))
	counts := 0
	for k := range v.GetStringMap("corpus") {
		counts++
		c := k
		logrus.Infof("[corpus] Type of corpus.%s.reply: %T", c, v.Get(fmt.Sprintf("corpus.%s.reply", c)))
		scene := func(ctx *zero.Ctx) bool {
			switch v.GetString(fmt.Sprintf("corpus.%s.scene", c)) {
			case "a": // 全部
				return true
			case "g": // 群
				return v.GetString(fmt.Sprintf("corpus.%s.scene", c)) == "g" && ctx.Event.DetailType == "group"
			case "p": // 私聊
				return v.GetString(fmt.Sprintf("corpus.%s.scene", c)) == "p" && ctx.Event.DetailType == "private"
			default:
				return false
			}
		}
		engine.OnRegex(v.GetString(fmt.Sprintf("corpus.%s.regexp", c)), scene).Handle(func(ctx *zero.Ctx) {
			time.Sleep(time.Second * time.Duration(v.GetInt64(fmt.Sprintf("corpus.%s.delay", c))))
			switch v.Get(fmt.Sprintf("corpus.%s.reply", c)).(type) {
			case string:
				ctx.Send(message.Text(v.GetString(fmt.Sprintf("corpus.%s.reply", c))))
			case []string, []any:
				ctx.Send(message.Text(v.GetString(fmt.Sprintf("corpus.%s.reply.%d", c, rand.Intn(len(v.GetStringSlice(fmt.Sprintf("%s.reply", c))))))))
			}
		})
	}
	logrus.Infof("[corpus] 语料库注册 %d 条", counts)
}

func init() {
	logrus.Infoln("[corpus] 读取语料库配置")
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetConfigFile("./config.yaml")
	v.ReadInConfig()
	v.WatchConfig()

	initOnRegex()
	v.OnConfigChange(func(in fsnotify.Event) {
		engine.Delete()
		initOnRegex()
	})
}
