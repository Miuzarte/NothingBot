package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jaypipes/ghw"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// 运行状态
func checkInfo(ctx gocqMessage) {
	match := ctx.regexpMustCompile(`检查身体|运行状态`)
	if len(match) > 0 && ctx.isToMe() {
		product, _ := ghw.Product()
		cpuInfo, _ := cpu.Info()
		memInfo, _ := mem.VirtualMemory()
		gpu, _ := ghw.GPU()
		cpuUtilization, _ := cpu.Percent(time.Second, false)
		s := fmt.Sprintf(
			`%s  %s
%s (%.2f%%)
%d / %d MB (%.2f%%)
%s
NothingBot运行时长：%s`,
			strings.ReplaceAll(product.Vendor, ", Ltd.", ""), product.Name,
			strings.ReplaceAll(cpuInfo[0].ModelName, "             ", ""), cpuUtilization[0],
			memInfo.Used/1024/1024, memInfo.Total/1024/1024, float64(memInfo.Used)/float64(memInfo.Total)*100,
			gpu.GraphicsCards[0].DeviceInfo.Product.Name,
			timeFormat(time.Now().Unix()-startTime))
		ctx.sendMsg(s)
	}
}
