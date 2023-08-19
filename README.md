# NothingBot
### There is Nothing, create your Everything.

~~高情商：扩展性很强~~

学习go的副产物，💩⛰️中的💩⛰️

~~有点越来越看不懂了~~

## 目前支持的功能

### 语料库（自动回复）

参照[Tsuk1ko/cq-picsearcher-bot](https://github.com/Tsuk1ko/cq-picsearcher-bot)的语料库实现，在其基础上多了并发、延迟发送

### 聊天相关

~~大部分功能都没什么用~~ 写着玩

**喊话超级用户：**

`@Bot@Bot{message}` 转发喊话消息至Bot管理员

**撤回消息记录：**

`让我康康[@群友|QQ号]撤回了什么` 输出群内撤回的消息集合（可过滤）

**谁at我：**

`谁{@|at|AT|艾特}{我|@群友|QQ号}` 输出群内at过某人的消息集合

**注入消息：**

`@Botrun{text}` 输出相应消息，支持CQ码

**回复我：**

`@Bot回复我[text]` 回复对应消息，支持CQ码

**运行状态：**

`[@Bot]{检查身体|运行状态}` 输出宿主机硬件信息、运行时长

### 哔哩哔哩相关

**动态推送：**

通过调用t.bilibili.com的新动态检测接口，3s拉取一次更新实现低延迟推送

**直播推送：**

通过建立直播间弹幕ws连接实现监听开播，[录播姬](https://github.com/BililiveRecorder/BililiveRecorder)同款

数据包解码[danmaku.go](danmaku.go)由[@moxcomic](https://github.com/moxcomic)编写

**链接解析：**

暂时只做了空间、动态、视频、专栏、直播的（直链/短链）解析

**快捷搜索：（working）**

`[Bb]搜(视频|番剧|影视|直播|直播间|主播|专栏|话题|用户|相簿)[\s:：]?(.*)` 通过合并转发输出结果

### 启动参数

**可选：**

- `-c` ：配置文件路径，默认"./config.yaml"

### TODO:

1. [ ] 卡拉彼丘，启动！

110. [ ] 摸鱼

111. [ ] 哔哩视频总结

998. [ ] 语料库捕获组

999. [ ] 推送at优化、at全体成员

[@moxcomic](https://github.com/moxcomic) :

![💩⛰️](https://github.com/Miuzarte/NothingBot/assets/66856838/98eb9a3e-c27c-4d08-8182-2332cf956198)
