package main

import (
	"NothinBot/EasyBot"
	"fmt"
	"strings"
	"time"

	"github.com/jaypipes/ghw"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// 运行状态
func checkInfo(ctx *EasyBot.CQMessage) {
	match := ctx.RegexpMustCompile(`检查身体|运行状态`)
	if len(match) > 0 && ctx.IsToMe() {
		product, _ := ghw.Product()
		cpuInfo, _ := cpu.Info()
		memInfo, _ := mem.VirtualMemory()
		gpus, _ := ghw.GPU()
		cpuUtilization, _ := cpu.Percent(time.Second, false)
		s := fmt.Sprintf(`[NothingBot]
%s  %s
%s (%.2f%%)
%.2f / %.2f MB (%.2f%%)
%s
运行时长：%s`,
			strings.ReplaceAll(product.Vendor, ", Ltd.", ""), product.Name,
			strings.ReplaceAll(cpuInfo[0].ModelName, "             ", ""), cpuUtilization[0],
			float64(memInfo.Used)/1024.0/1024.0, float64(memInfo.Total)/1024.0/1024.0, float64(memInfo.Used)/float64(memInfo.Total)*100,
			func() (s string) {
				for i, gpu := range gpus.GraphicsCards {
					name := gpu.DeviceInfo.Product.Name
					if !strings.Contains(name, "NVIDIA") && !strings.Contains(name, "AMD") {
						break
					}
					if s != "" {
						s += "\n"
					}
					s += fmt.Sprint("GPU", i, ": ") + name
				}
				return
			}(),
			timeFormat(bot.GetRunningTime()))
		ctx.SendMsg(s)
	}
}
