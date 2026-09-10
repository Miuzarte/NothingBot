package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"

	"github.com/Miuzarte/SimpleLog"
	"github.com/fsnotify/fsnotify"
	"github.com/redis/rueidis"
	"github.com/spf13/viper"
)

var (
	runTime  = time.Now() // 运行时间
	connTime time.Time    // 连接时间
	stopTime time.Time    // 停止时间

	log = SimpleLog.New("[NothingBot]", true, true)

	config = Config{
		UpdateChanAdmin:     make(chan int, 1),
		UpdateChanFileEvent: make(chan fsnotify.Event, 1),
		AutoReload:          true,
		LastChange:          time.Now(),
		UpdateCount:         0,

		Viper: viper.New(),
	}

	redisClient RedisClient
	onebot      = EasyOnebot.New()
)

var logFs *SafeFile

func InitLog() {
	if config.Log.Level > 6 {
		config.Log.Level = 6
	}
	l := SimpleLog.Level(config.Log.Level)
	onebot.SetLogLevel(l)
	log.SetLevel(l)
	if config.Log.Dir != "" && logFs == nil && // 无热更新
		!env.Testing {
		logFs = CreateLogFile(config.Log.Dir)
		log.AddOutput(logFs)
	}
}

func InitRedis() {
	if config.Global.RedisUrl == "" {
		log.Fatal("[main] no redis url provided")
	}
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{config.Global.RedisUrl},
		// DisableCache: true, // Disable Client-Side Caching
	})
	redisClient.Client = client
	if err != nil {
		log.Fatal("[main] failed to init redis: ", err)
	}
}

func initMain() {
	// 命令控制
	onebot.AddMatcher("admincommand", EasyOnebot.NewMatcher().
		OnTypeL1(event.TYPE_L1_MESSAGE).
		OnTypeL2(event.TYPE_L2_MESSAGE_PRIVATE).
		IsSuperuser().
		Do(func(ctx *EasyOnebot.Ctx) {
			if len(ctx.Event.RawMessage) < 1 || ctx.Event.RawMessage[0] != '/' {
				return
			}

			lowerRm := strings.ToLower(ctx.Event.RawMessage)
			switch {
			case strings.Contains(lowerRm, "/reload config"):
				config.UpdateChanAdmin <- ctx.Event.MessageId

			case strings.Contains(lowerRm, "/autoreload toggle"):
				config.AutoReload = !config.AutoReload
				ctx.SendMsg(fmt.Sprintf("autoreload: %v", config.AutoReload))

			case strings.Contains(lowerRm, "/shutdown"):
				// TODO

			}
		}),
	)

	if env.Testing { // unit test
		onebot.AddMatcher("test", EasyOnebot.NewMatcher().
			OnTypeL1(event.TYPE_L1_MESSAGE).
			OnFunc(func(ctx *EasyOnebot.Ctx) bool {
				lowerRm := strings.ToLower(ctx.Event.RawMessage)
				return strings.Contains(lowerRm, "stop") && strings.Contains(lowerRm, "test")
			}).
			Do(func(ctx *EasyOnebot.Ctx) {
				onebot.Stop()
				os.Exit(0)
			}),
		)
	}
}

var configLoopStop = make(chan struct{})

func configLoop() {
	for {
		select {
		case e, ok := <-config.UpdateChanFileEvent:
			if !ok {
				return
			}
			tn := time.Now()
			if !e.Has(fsnotify.Write) {
				log.Warn("[main] unexpected config file event: ", e)
				continue
			}

			if tn.Sub(config.LastChange) < time.Second {
				log.Debug("[main] config reload ignored: ", e.Op)
				continue
			}
			config.LastChange = tn
			if !config.AutoReload {
				log.Debug("[main] config reload skipped: ", e.Op)
				continue
			}

			log.Info("[main] config reload triggered by file event: ", e.Op)
			config.UpdateCount++
			go ReInit() // 阻塞太久会导致处理下一个信号时超时 time.Second

		case msg, ok := <-config.UpdateChanAdmin:
			if !ok {
				return
			}
			log.Info("[main] config reload triggered by admin: ", msg)
			config.UpdateCount++
			go ReInit()

		case <-configLoopStop:
			return
		}
	}
}

var moduleMain = Module{
	ModuleMeta: ModuleMeta{
		Name:   "main",
		Hidden: true,
	},
}

// 模块文件名都以大写开头, init() 调用顺序必定都早于 main.go
func init() {
	NoBuildPrintFile("main.go")

	moduleMain.Init = initMain
	moduleMain.AfterInit = func() { go configLoop() }
	moduleMain.AtExit = func() { close(configLoopStop) }
	modules.Add(&moduleMain)

	Init()
}

// Init 运行前初始化
func Init() {
	modules.Sort()
	InitConfig()
	config.Unmarshal()
	InitLog()
	InitRedis()
	var t time.Time
	var ts time.Duration
	for _, name := range modules.SortedKeys {
		m := modules.M[name]
		if m.Disable {
			continue
		}

		if !m.RWMutex.TryLock() {
			log.Panicf("[main] [FIXME] module %s failed to lock in initialization", name)
		}

		if m.Init != nil {
			t = time.Now()
			m.Init()
			if ts = time.Since(t); ts >= time.Millisecond {
				log.Debugf("[main] module %s initialized in %s", name, ts)
			} else {
				log.Debugf("[main] module %s initialized", name)
			}
		}

		m.RWMutex.Unlock()
	}
}

// AfterInit 连接成功获取到 uin 后初始化
func AfterInit() {
	var t time.Time
	var ts time.Duration
	for _, name := range modules.SortedKeys {
		m := modules.M[name]
		if m.Disable {
			continue
		}

		if !m.RWMutex.TryLock() {
			log.Panicf("[main] [FIXME] module %s failed to lock in after-initialization", name)
		}

		if m.AfterInit != nil {
			t = time.Now()
			m.AfterInit()
			if ts = time.Since(t); ts >= time.Millisecond {
				log.Debugf("[main] module %s after-initialized in %s", name, ts)
			} else {
				log.Debugf("[main] module %s after-initialized", name)
			}
		}

		m.RWMutex.Unlock()
	}
}

// ReInit 重载配置文件后初始化
func ReInit() {
	config.Unmarshal() // 同步配置到内存
	InitLog()          // 更新日志配置
	var t time.Time
	var ts time.Duration
	for _, name := range modules.SortedKeys {
		m := modules.M[name]
		if m.Disable {
			continue
		}

		if !m.RWMutex.TryLock() {
			log.Infof("[main] module %s is still busy...", name)
			m.RWMutex.Lock()
		}

		if m.ReInit != nil {
			t = time.Now()
			m.ReInit()
			if ts = time.Since(t); ts >= time.Millisecond {
				log.Debugf("[main] module %s re-initialized in %s", name, ts)
			} else {
				log.Debugf("[main] module %s re-initialized", name)
			}
		}

		m.RWMutex.Unlock()
	}
}

// AtExit 退出时执行
func AtExit() {
	var t time.Time
	var ts time.Duration
	for _, name := range modules.SortedKeys {
		m := modules.M[name]

		if !m.RWMutex.TryLock() {
			log.Infof("[main] module %s is still working...", name)
			m.RWMutex.Lock()
		}

		if m.AtExit != nil {
			t = time.Now()
			m.AtExit()
			if ts = time.Since(t); ts >= time.Millisecond {
				log.Debugf("[main] module %s exited in %s", name, ts)
			} else {
				log.Debugf("[main] module %s exited", name)
			}
		}

		m.RWMutex.Unlock()
	}
}

func InitBot() *EasyOnebot.Bot {
	if config.Onebot.WsUrl == "" {
		log.Fatal("[main] no wsurl provided")
	}
	onebot.SetRedisClient(redisClient.Client)
	if config.Onebot.RedisExpire != "" {
		dur, err := time.ParseDuration(config.Onebot.RedisExpire)
		if err == nil {
			if dur >= 0 {
				onebot.SetRedisExpire(dur, dur/7/24)
			} else {
				log.Error("[main] redis expire should be positive")
			}
		} else {
			log.Error("[main] failed to parse redis expire: ", err)
		}
	}

	return onebot.
		SetWsUrl(config.Onebot.WsUrl).
		SetToken(config.Onebot.Token).
		SetOnlineNotify(config.Onebot.OnlineNotify).
		SetOfflineNotify(config.Onebot.OfflineNotify).
		// SetOnlineNotify(false).
		// SetOfflineNotify(false).
		SetSuperusers(config.Onebot.Superusers).
		SetNicknames(config.Onebot.Nicknames).
		SetFilterStranger(config.Onebot.Filter.Users).
		SetFilterGroup(config.Onebot.Filter.Groups).
		SetMsgProcessEnabled(false) // 初始化阶段暂停消息处理, AfterInit 完成后开启
}

// for unit testing
func runBot() {
	log.Debug("[main] total init time: ", time.Since(runTime))
	InitBot().Run()
	connTime = time.Now() // TODO: move to redis (?
	modules.IsRunning.Store(true)
	AfterInit()
	onebot.SetMsgProcessEnabled(true)
	log.Debug("[main] after-initialized time: ", time.Since(connTime))
}

func main() {
	runBot()
	defer func() {
		if redisClient.Client != nil {
			redisClient.Client.Close()
		}
	}()

	signalCh := make(chan os.Signal, 1) // 捕获 Ctrl+C
	defer close(signalCh)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	sig, ok := <-signalCh
	if !ok {
		log.Panic("[main] signalCh closed unexpectedly")
	}
	log.Debug("[main] signal received: ", sig)
	signal.Stop(signalCh) // 再次按下时强制退出, 避免长时间无法结束

	config.OnConfigChange(nil)
	close(config.UpdateChanAdmin)
	close(config.UpdateChanFileEvent)

	stopTime = time.Now()
	AtExit()
	log.Debug("[main] exited in ", time.Since(stopTime))

	stopTime = time.Now()
	onebot.Stop()
	log.Debug("[main] stopped in ", time.Since(stopTime))

	connDuration, connectCount, hbCount, lostCount := onebot.Statistics()
	log.Debug("[main] 此次连接持续: ", connDuration)
	log.Debug("[main] 连接次数: ", connectCount)
	log.Debug("[main] 心跳包: ", hbCount)
	log.Debug("[main] 心跳丢包: ", lostCount)

	tn := time.Now()
	log.Debug("[main] 运行时长: ", tn.Sub(runTime))
	log.Debug("[main] 连接时长: ", tn.Sub(connTime))
	log.Debug("[main] 配置重载计数: ", config.UpdateCount)
}
