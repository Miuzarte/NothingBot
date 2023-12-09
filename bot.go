package main

import (
	"NothinBot/EasyBot"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	callSUMsgList   []*EasyBot.CQMessage
	callSUMsgUnread int
)

// Bot内置逻辑
func checkBotInternal(ctx *EasyBot.CQMessage) {
	var match [][]string
	//连续at两次获取帮助, 带文字则视为喊话超级用户
	match = ctx.RegFindAllStringSubmatch(
		regexp.MustCompile(
			fmt.Sprintf(
				`^\[CQ:at,qq=%d]\s*\[CQ:at,qq=%d]\s*(.*)$`, bot.GetSelfId(), bot.GetSelfId(),
			),
		),
	)
	if len(match) > 0 {
		call := match[0][1]
		if len(call) > 0 { //记录喊话
			callSUMsgList = append(callSUMsgList, ctx)
			callSUMsgUnread++
			ctx.SendMsgReply("[NothingBot] 已记录此条喊话并通知超级用户")
			bot.Log2SU.Info("收到一条新的喊话，未读", callSUMsgUnread)
		} else { //输出帮助
			ctx.SendForwardMsg(
				EasyBot.FastNewForwardMsg(
					"NothingBot", bot.GetSelfId(), 0, 0, []string{
						"[NothinBot]" +
							"\n帮助信息" +
							"\ngithub.com/Miuzarte/NothingBot" +
							"\n符号：" +
							"\n{}: 必要参数" +
							"\n[]: 可选参数" +
							"\n|: 或",

						"获取帮助信息：" +
							"\n“{@Bot}{@Bot}”" +
							"\n（“@Bot @Bot ”）" +
							"\n输出帮助信息",

						"喊话超级用户：" +
							"\n“{@Bot}{@Bot}{message}”" +
							"\n（“@Bot @Bot 出bug辣”）" +
							"\n转发喊话消息至Bot管理员",

						"at消息记录：" +
							"\n“谁{@|at|AT|艾特}{我|@群友|QQ号}”" +
							"\n（“谁at我”）" +
							"\n输出群内at过某人的消息集合",

						fmt.Sprint("(Enable: ", recallEnable, ")撤回消息记录：") +
							"\n“让我康康[@群友|QQ号]撤回了什么”" +
							"\n（“让我康康撤回了什么”）" +
							"\n输出群内撤回的消息集合（可过滤）",

						"哔哩哔哩链接解析：" +
							"\n短链、动态、视频、专栏、空间、直播间" +
							"\n（“space.bilibili.com/59442895”）" +
							"\n解析内容信息，同时如果视频存在官方AI总结的话会一并输出",

						"哔哩哔哩视频、专栏、动态总结：" +
							"\n“总结一下+内容链接”" +
							"\n（“总结一下www.bilibili.com/read/cv19661826”）" +
							"\n总结视频字幕（无字幕时调用剪映语言识别接口，准确率较低）、专栏正文、动态正文",

						"哔哩哔哩快捷搜索：" +
							"\n“B搜{视频|番剧|影视|直播间|直播|主播|专栏|用户}{keywords}”" +
							"\n（“B搜用户謬紗特”）" +
							"\n取决于类别，B站只会返回最多20或30条结果",

						"注入消息：" +
							"\n“{@Bot}run{text}”" +
							"\n（“@Bot run[CQ:at,​qq=" + fmt.Sprint(bot.GetSelfId()) + "]”）" +
							"\n输出相应消息，支持CQ码",

						"回复：" +
							"\n“{@Bot}回复我[text]”" +
							"\n（“@Bot 回复我114514”）" +
							"\n回复对应消息，支持CQ码",

						"运行状态：" +
							"\n“{@Bot}{检查身体|运行状态}”" +
							"\n（“检查身体”）" +
							"\n输出NothingBot运行信息",

						fmt.Sprint("(Enable: ", pixivEnable, ")pixiv：") +
							"\n“{(看看|康康|kk)(p|p站|pid|pixiv)}{pid}”" +
							"\n（“kkp111158204”）",

						fmt.Sprint("(Enable: ", setuEnable, ")setu：") +
							"\n“{@Bot}来{点|一点|几张|几份|.*张|.*份}[tag][的]色图|{@Bot}[tag][的]色图来{点|一点|几张|几份|.*张|.*份}”" +
							"\n（“@Bot来点碧蓝档案色图”）" +
							"\n“点”、“一点”、“几张”、“几份”会取一个[3,6]的随机数，“来张”、“来份”不含数量则为1，“xx张”，“xx份”支持[1,20]的阿拉伯数字、汉字大小写数字，可以使用 &(和) 和 |(或) 将多个关键词进行组合， | 的优先级永远高于 & ",
					}...,
				),
			)
		}
	}
	//发送/清空收件箱
	match = ctx.RegFindAllStringSubmatch(regexp.MustCompile(`^(清空)?(喊话列表|收件箱)$`))
	if len(match) > 0 && ctx.IsPrivateSU() {
		callSUMsgUnread = 0    //清零未读
		if match[0][1] == "" { //发送
			sort.Slice(
				callSUMsgList, func(i, j int) bool { //根据msg的时间戳由大到小排序
					return callSUMsgList[i].Event.Time > callSUMsgList[j].Event.Time
				},
			)
			callSUMsgLen := len(callSUMsgList)
			if callSUMsgLen == 0 {
				ctx.SendMsg("[NothingBot] [Info] 收件箱为空！")
				return
			}
			if callSUMsgLen > 100 { //超过100条合并转发放不下
				callSUMsgLen = 100
			}
			var forwardMsg EasyBot.CQForwardMsg
			for i := 0; i < callSUMsgLen; i++ {
				callSUMsg := callSUMsgList[i]
				name := fmt.Sprintf(
					`(%s)%s  (%d)`,
					callSUMsg.Extra.TimeFormat,
					callSUMsg.GetCardOrNickname(),
					callSUMsg.GroupID,
				)
				content := strings.ReplaceAll(callSUMsg.GetRawMessageOrMessage(), "CQ:at,", "CQ:at,​") //插入零宽空格阻止CQ码解析
				forwardMsg = EasyBot.AppendForwardMsg(
					forwardMsg, EasyBot.NewCustomForwardNode(
						name, callSUMsg.UserID, content, int64(callSUMsg.Event.Time),
						int64(callSUMsg.MessageSeq),
					),
				)
			}
			ctx.SendForwardMsg(forwardMsg)
		} else if match[0][1] == "清空" { //清空
			callSUMsgList = []*EasyBot.CQMessage{}
			ctx.SendMsg("[Info] [NothingBot] 已清空")
		}
	}
	//注入消息
	match = ctx.Unescape().RegFindAllStringSubmatch(regexp.MustCompile(`run(.*)`))
	if len(match) > 0 && ctx.IsToMe() {
		ctx.SendMsg(match[0][1])
	}
	//回复我
	match = ctx.Unescape().RegFindAllStringSubmatch(regexp.MustCompile(`回复我(.*)?`))
	if len(match) > 0 && ctx.IsToMe() {
		if match[0][1] == "" {
			ctx.SendMsgReply("回复你")
		} else {
			ctx.SendMsgReply(match[0][1])
		}
	}
}
