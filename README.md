# NothingBot
### There is Nothing, create your Everything.

~~高情商：扩展性很强~~

学习go的副产物，💩⛰️中的💩⛰️

~~有点越来越看不懂了~~

## 目前支持的功能

### 语料库（自动回复）

参照[Tsuk1ko/cq-picsearcher-bot](https://github.com/Tsuk1ko/cq-picsearcher-bot)的语料库实现，在其基础上多了并发、延迟发送

### 哔哩哔哩相关

**动态推送：**

通过调用t.bilibili.com的新动态检测接口，3s拉取一次更新实现低延迟推送

**直播推送：**

通过建立直播间弹幕ws连接实现监听开播，[录播姬](https://github.com/BililiveRecorder/BililiveRecorder)同款

数据包解码[danmaku.go](danmaku.go)由[@moxcomic](https://github.com/moxcomic)编写

**链接解析：**

暂时只做了空间、动态、视频、专栏、直播的（直链/短链）解析

**快捷搜索：（working）**

`[Bb]搜(视频|番剧|影视|直播|直播间|主播|专栏|话题|用户|相簿)[\s:：]?(.*)`

### TODO:

1. [ ] 语料库捕获组

2. [ ] 推送at优化、at全体成员

[@moxcomic](https://github.com/moxcomic) :

![💩⛰️](https://github.com/Miuzarte/NothingBot/assets/66856838/98eb9a3e-c27c-4d08-8182-2332cf956198)
