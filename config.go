package main

import (
	"bytes"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/fsnotify/fsnotify"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// 本文件的日志 scope
var logConfig = logger.New("config")

//go:embed defaultConfig.toml
var configDataDefault []byte

func InitConfig() {
	var err error
	switch {
	case env.Testing || env.Debugging:
		logConfig.Debug().Msg("testing/debugging mode, using default")
		config.SetConfigType("toml")
		err = config.ReadConfig(bytes.NewReader(configDataDefault))
		if err != nil {
			logConfig.Panic().
				Err(err).
				Msg("failed to read default")
		}

	default:
		path := ""
		if env.NoBuild {
			path = filepath.Join(env.WorkDir, "config.toml")
		} else {
			path = filepath.Join(env.XDir, "config.toml")
		}
		logConfig.Info().
			Str("path", path).
			Msg("reading config")
		config.SetConfigFile(path)
		err = config.ReadInConfig()
		if err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if !errors.As(err, &configFileNotFoundError) {
				f, err := os.Create(path)
				if err != nil {
					logConfig.Panic().
						Err(err).
						Msg("failed to create file")
				}
				_, err = f.Write(configDataDefault)
				if err != nil {
					logConfig.Panic().
						Err(err).
						Msg("failed to write default")
				}
				err = f.Close()
				if err != nil {
					logConfig.Panic().
						Err(err).
						Msg("failed to close file")
				}
				logConfig.Info().
					Str("path", path).
					Msg("default config created")
				logConfig.Info().Msg("edit the file then restart the bot")
				os.Exit(0)
			}
		}
		config.WatchConfig()
		config.OnConfigChange(func(e fsnotify.Event) {
			config.UpdateChanFileEvent <- e
		})

	}
}

type Config struct {
	UpdateChanAdmin     chan int
	UpdateChanFileEvent chan fsnotify.Event
	AutoReload          bool
	LastChange          time.Time
	UpdateCount         int

	Log struct {
		Level int
		Dir   string
	}
	Global struct {
		RedisUrl string
		Proxy    string
	}
	Onebot struct {
		WsUrl         string
		Token         string
		RedisExpire   string
		OnlineNotify  bool
		OfflineNotify bool
		Superusers    []int
		Nicknames     []string
		Filter        struct {
			Users  []int
			Groups []int
		}
	}
	Modules map[string]any

	*viper.Viper
}

func (c *Config) Unmarshal() {
	var err error
	err = c.UnmarshalKey("log", &c.Log)
	if err != nil {
		logConfig.Fatal().
			Err(err).
			Msg("failed to unmarshal 'log'")
	}
	// logConfig.Trace().Msgf("log: %v", c.Log)
	err = c.UnmarshalKey("global", &c.Global)
	if err != nil {
		logConfig.Fatal().
			Err(err).
			Msg("failed to unmarshal 'global'")
	}
	// logConfig.Trace().Msgf("global: %v", c.Global)
	err = c.UnmarshalKey("onebot", &c.Onebot)
	if err != nil {
		logConfig.Fatal().
			Err(err).
			Msg("failed to unmarshal 'onebot'")
	}
	// logConfig.Trace().Msgf("onebot: %v", c.Onebot)
	err = c.UnmarshalKey("modules", &c.Modules)
	if err != nil {
		logConfig.Fatal().
			Err(err).
			Msg("failed to unmarshal 'modules'")
	}
	// logConfig.Trace().Msgf("modules: %v", c.Modules)

	os.Setenv("HTTP_PROXY", c.Global.Proxy)
	os.Setenv("HTTPS_PROXY", c.Global.Proxy)
}

type ModuleId string

func (mid ModuleId) String() string {
	return string(mid)
}

func (mid ModuleId) WithSuffix(suf string) ModuleId {
	return mid + "_" + ModuleId(suf)
}

var ErrModuleConfigNotFound = errors.New("module config not found")

func (c *Config) DecodeModule(moduleId ModuleId, output any) error {
	data, ok := c.Modules[strings.ToLower(moduleId.String())]
	if !ok || data == nil {
		return wrapErr(ErrModuleConfigNotFound, moduleId)
	}
	return mapstructure.Decode(data, output)
}

func (c *Config) GetModule(moduleId ModuleId) (any, error) {
	data, ok := c.Modules[strings.ToLower(moduleId.String())]
	if !ok || data == nil {
		return nil, wrapErr(ErrModuleConfigNotFound, moduleId)
	}
	return data, nil
}

type List struct {
	Groups []int
	Users  []int
}

func (l *List) White(ctx *EasyOnebot.Ctx) bool {
	switch ctx.Event.TypeL2 {
	case "group":
		return slices.Contains(l.Groups, ctx.Event.GroupId)
	case "private":
		return slices.Contains(l.Users, ctx.Event.UserId)
	default:
		logger.FakePanic(&logConfig, ctx.Event.TypeL2)
		return false
	}
}

func (l *List) Black(ctx *EasyOnebot.Ctx) bool {
	return !l.White(ctx)
}
