package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

func TestToml(t *testing.T) {
	v := viper.New()
	v.SetConfigFile("defaultConfig.toml")
	err := v.ReadInConfig()
	if err != nil {
		t.Error(err)
	}
	v.WriteConfigAs("defaultConfig.yaml")
	v.WriteConfigAs("defaultConfig.json")
	v.WriteConfigAs("defaultConfig.ini")
	v.WriteConfigAs("defaultConfig.hcl")
}

func TestConfigRead(t *testing.T) {
	v := viper.New()
	v.SetConfigFile("defaultConfig.toml")
	err := v.ReadInConfig()
	if err != nil {
		t.Error(err)
	}
	fmt.Println(v.AllSettings())
}

func TestDefaultConfig(t *testing.T) {
	fmt.Printf("%+v", config)
}

func TestMapstructure(t *testing.T) {
	InitConfig()
	config.Unmarshal()
	all := config.AllSettings()
	fmt.Println(all)
	log := all["log"].(map[string]any)
	fmt.Println(log)
	mapstructure.Decode(log, &config.Log)
	fmt.Println(config.Log)
	fmt.Println(config.Log.Level)
}

func TestNumber(t *testing.T) {
	toml := `
[test]
number1 = 123
number2 = 123.0
duration1 = "3s"
duration2 = "3m3s"
duration3 = "3h3m3s"
duration4 = "3.3s"
duration5 = "3.3m3.3s"
duration6 = "3.3h3.3m3.3s"`
	v := viper.New()
	v.SetConfigType("toml")
	v.ReadConfig(strings.NewReader(toml))
	t.Log(v.Get("test.number1"))
	t.Log(v.GetInt("test.number1"))
	t.Log(v.GetFloat64("test.number1"))
	t.Log(v.Get("test.number2"))
	t.Log(v.GetInt("test.number2"))
	t.Log(v.GetFloat64("test.number2"))
	t.Log(v.GetDuration("test.duration1"))
	t.Log(v.GetDuration("test.duration2"))
	t.Log(v.GetDuration("test.duration3"))
	t.Log(v.GetDuration("test.duration4"))
	t.Log(v.GetDuration("test.duration5"))
	t.Log(v.GetDuration("test.duration6"))
}
