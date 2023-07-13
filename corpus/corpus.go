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
	for k := range v.AllSettings() {
		c := k
		fmt.Printf("%s.regexp: %T\n", c, v.Get(fmt.Sprintf("%s.reply", c)))
		engine.OnRegex(v.GetString(fmt.Sprintf("%s.regexp", c))).Handle(func(ctx *zero.Ctx) {
			time.Sleep(time.Second * time.Duration(v.GetInt64(fmt.Sprintf("%s.delay", c))))
			switch v.Get(fmt.Sprintf("%s.reply", c)).(type) {
			case string:
				ctx.SendChain(message.Text(v.GetString(fmt.Sprintf("%s.reply", c))))
			case []string, []any:
				ctx.SendChain(message.Text(v.GetString(fmt.Sprintf("%s.reply.%d", c, rand.Intn(len(v.GetStringSlice(fmt.Sprintf("%s.reply", c))))))))
			}
		})
	}
}

func init() {
	logrus.Infoln("[main] 读取语料库配置")
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
