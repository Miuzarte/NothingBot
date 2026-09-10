package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/event"

	"github.com/fsnotify/fsnotify"
	"github.com/redis/rueidis"
	"github.com/spf13/viper"
)

// 本文件的日志 scope
var logMain = logger.New("main")

var (
	runTime  = time.Now() // 运行时间
	connTime time.Time    // 连接时间
	stopTime time.Time    // 停止时间

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

	logOnebot = logger.New("EasyOneBot") // 注入给 EasyOnebot, 不直接使用
)

var logFs *SafeFile

func InitLog() {
	logger.SetLevel(logger.Level(config.Log.Level))
	if config.Log.Dir != "" && logFs == nil && // 无热更新
		!env.Testing {
		logFs = CreateLogFile(config.Log.Dir)
		logger.AddOutput(logFs)
	}
}

func InitRedis() {
	if config.Global.RedisUrl == "" {
		logMain.Fatal().Msg("no redis url provided")
	}
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{config.Global.RedisUrl},
		// DisableCache: true, // Disable Client-Side Caching
	})
	redisClient.Client = client
	if err != nil {
		logMain.Fatal().
			Err(err).
			Msg("failed to init redis")
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
				logMain.Warn().
					Any("event", e).
					Msg("unexpected config file event")
				continue
			}

			if tn.Sub(config.LastChange) < time.Second {
				logMain.Debug().
					Str("op", e.Op.String()).
					Msg("config reload ignored")
				continue
			}
			config.LastChange = tn
			if !config.AutoReload {
				logMain.Debug().
					Str("op", e.Op.String()).
					Msg("config reload skipped")
				continue
			}

			logMain.Info().
				Str("op", e.Op.String()).
				Msg("config reload triggered by file event")
			config.UpdateCount++
			go ReInit() // 阻塞太久会导致处理下一个信号时超时 time.Second

		case msg, ok := <-config.UpdateChanAdmin:
			if !ok {
				return
			}
			logMain.Info().
				Int("mid", msg).
				Msg("config reload triggered by admin")
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
			logMain.Panic().
				Str("module", string(name)).
				Msg("[FIXME] module failed to lock in initialization")
		}

		if m.Init != nil {
			t = time.Now()
			m.Init()
			if ts = time.Since(t); ts >= time.Millisecond {
				logMain.Debug().
					Str("module", string(name)).
					Dur("cost", ts).
					Msg("module initialized")
			} else {
				logMain.Debug().
					Str("module", string(name)).
					Msg("module initialized")
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
			logMain.Panic().
				Str("module", string(name)).
				Msg("[FIXME] module failed to lock in after-initialization")
		}

		if m.AfterInit != nil {
			t = time.Now()
			m.AfterInit()
			if ts = time.Since(t); ts >= time.Millisecond {
				logMain.Debug().
					Str("module", string(name)).
					Dur("cost", ts).
					Msg("module after-initialized")
			} else {
				logMain.Debug().
					Str("module", string(name)).
					Msg("module after-initialized")
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
			logMain.Info().
				Str("module", string(name)).
				Msg("module is still busy")
			m.RWMutex.Lock()
		}

		if m.ReInit != nil {
			t = time.Now()
			m.ReInit()
			if ts = time.Since(t); ts >= time.Millisecond {
				logMain.Debug().
					Str("module", string(name)).
					Dur("cost", ts).
					Msg("module re-initialized")
			} else {
				logMain.Debug().
					Str("module", string(name)).
					Msg("module re-initialized")
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
			logMain.Info().
				Str("module", string(name)).
				Msg("module is still working")
			m.RWMutex.Lock()
		}

		if m.AtExit != nil {
			t = time.Now()
			m.AtExit()
			if ts = time.Since(t); ts >= time.Millisecond {
				logMain.Debug().
					Str("module", string(name)).
					Dur("cost", ts).
					Msg("module exited")
			} else {
				logMain.Debug().
					Str("module", string(name)).
					Msg("module exited")
			}
		}

		m.RWMutex.Unlock()
	}
}

func InitBot() *EasyOnebot.Bot {
	if config.Onebot.WsUrl == "" {
		logMain.Fatal().Msg("no wsurl provided")
	}
	onebot.SetRedisClient(redisClient.Client)
	if config.Onebot.RedisExpire != "" {
		dur, err := time.ParseDuration(config.Onebot.RedisExpire)
		if err == nil {
			if dur >= 0 {
				onebot.SetRedisExpire(dur, dur/7/24)
			} else {
				logMain.Error().Msg("redis expire should be positive")
			}
		} else {
			logMain.Error().
				Err(err).
				Msg("failed to parse redis expire")
		}
	}

	return onebot.
		SetLogger(&logOnebot).
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
	logMain.Debug().
		Dur("cost", time.Since(runTime)).
		Msg("total init time")
	InitBot().Run()
	connTime = time.Now() // TODO: move to redis (?
	modules.IsRunning.Store(true)
	AfterInit()
	onebot.SetMsgProcessEnabled(true)
	logMain.Debug().
		Dur("cost", time.Since(connTime)).
		Msg("after-initialized time")
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
		logMain.Panic().Msg("signalCh closed unexpectedly")
	}
	logMain.Debug().
		Any("signal", sig).
		Msg("signal received")
	signal.Stop(signalCh) // 再次按下时强制退出, 避免长时间无法结束

	config.OnConfigChange(nil)
	close(config.UpdateChanAdmin)
	close(config.UpdateChanFileEvent)

	stopTime = time.Now()
	AtExit()
	logMain.Debug().
		Dur("cost", time.Since(stopTime)).
		Msg("exited")

	stopTime = time.Now()
	onebot.Stop()
	logMain.Debug().
		Dur("cost", time.Since(stopTime)).
		Msg("stopped")

	connDuration, connectCount, hbCount, lostCount := onebot.Statistics()
	logMain.Debug().
		Dur("connDuration", connDuration).
		Msg("connection duration")
	logMain.Debug().
		Int("connectCount", connectCount).
		Msg("connect count")
	logMain.Debug().
		Int("hbCount", hbCount).
		Msg("heartbeat count")
	logMain.Debug().
		Int("lostCount", lostCount).
		Msg("heartbeat lost count")

	tn := time.Now()
	logMain.Debug().
		Dur("uptime", tn.Sub(runTime)).
		Msg("uptime")
	logMain.Debug().
		Dur("connDuration", tn.Sub(connTime)).
		Msg("connection duration")
	logMain.Debug().
		Int("configReloads", config.UpdateCount).
		Msg("config reload count")
}
