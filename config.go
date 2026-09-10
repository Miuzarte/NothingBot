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

	"github.com/Miuzarte/EasyOnebot"
	"github.com/fsnotify/fsnotify"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

//go:embed defaultConfig.toml
var configDataDefault []byte

func InitConfig() {
	var err error
	switch {
	case env.Testing || env.Debugging:
		log.Debug("[config] testing/debugging mode, using default")
		config.SetConfigType("toml")
		err = config.ReadConfig(bytes.NewReader(configDataDefault))
		if err != nil {
			log.Panic("[config] failed to read default: ", err)
		}

	default:
		path := ""
		if env.NoBuild {
			path = filepath.Join(env.WorkDir, "config.toml")
		} else {
			path = filepath.Join(env.XDir, "config.toml")
		}
		log.Info("[config] reading: ", path)
		config.SetConfigFile(path)
		err = config.ReadInConfig()
		if err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if !errors.As(err, &configFileNotFoundError) {
				f, err := os.Create(path)
				if err != nil {
					log.Panic("[config] failed to create file: ", err)
				}
				_, err = f.Write(configDataDefault)
				if err != nil {
					log.Panic("[config] failed to write default: ", err)
				}
				err = f.Close()
				if err != nil {
					log.Panic("[config] failed to close file: ", err)
				}
				log.Info("[config] default created: ", path)
				log.Info("[config] edit the file then restart the bot")
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
		Level uint32
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
		log.Fatal("[config] failed to unmarshal 'log': ", err)
	}
	// log.Trace("[config] log: ", c.Log)
	err = c.UnmarshalKey("global", &c.Global)
	if err != nil {
		log.Fatal("[config] failed to unmarshal 'global': ", err)
	}
	// log.Trace("[config] global: ", c.Global)
	err = c.UnmarshalKey("onebot", &c.Onebot)
	if err != nil {
		log.Fatal("[config] failed to unmarshal 'onebot': ", err)
	}
	// log.Trace("[config] onebot: ", c.Onebot)
	err = c.UnmarshalKey("modules", &c.Modules)
	if err != nil {
		log.Fatal("[config] failed to unmarshal 'modules': ", err)
	}
	// log.Trace("[config] modules: ", c.Modules)

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
		log.FakePanic(ctx.Event.TypeL2)
		return false
	}
}

func (l *List) Black(ctx *EasyOnebot.Ctx) bool {
	return !l.White(ctx)
}
