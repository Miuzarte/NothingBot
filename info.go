package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jaypipes/ghw"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

func checkInfo(ctx gocqMessage) {
	reg := regexp.MustCompile(fmt.Sprintf(`^(\[CQ:at\,qq=%d])?(检查身体|运行状态)$`, selfID)).FindAllStringSubmatch(ctx.message, -1)
	if len(reg) > 0 {
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
		sendMsgCTX(ctx, s)
	}
}
