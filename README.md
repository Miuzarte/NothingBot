# NothingBot
### There is Nothing, create your Everything.

学习go的副产物，💩⛰️中的💩⛰️

## 目前支持的功能

### 语料库（自动回复）

参照[Tsuk1ko/cq-picsearcher-bot](https://github.com/Tsuk1ko/cq-picsearcher-bot)的语料库实现，在其基础上多了并发、延迟发送

### 哔哩哔哩动态/直播推送

**动态：**

通过调用t.bilibili.com的新动态检测接口，3s拉取一次更新实现低延迟推送

**直播：**

通过建立直播间弹幕ws连接实现监听开播，[录播姬](https://github.com/BililiveRecorder/BililiveRecorder)同款

数据包解码[danmaku.go](danmaku.go)由[@moxcomic](https://github.com/moxcomic)编写

### TODO:

1. [ ] 哔哩哔哩链接解析

2. [ ] 语料库捕获组

3. [ ] 推送at优化、at全体成员

[@moxcomic](https://github.com/moxcomic) :

![💩⛰️](https://github.com/Miuzarte/NothingBot/assets/66856838/98eb9a3e-c27c-4d08-8182-2332cf956198)
