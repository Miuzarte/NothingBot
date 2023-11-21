# NothingBot
### There is Nothing, create your Everything.

~~高情商：扩展性很强~~

学习go的副产物，💩⛰️中的💩⛰️

~~有点越来越看不懂了~~

## 目前支持的功能

### 语料库（自动回复）

参照[Tsuk1ko/cq-picsearcher-bot](https://github.com/Tsuk1ko/cq-picsearcher-bot)的语料库实现

**在其基础上多了：**

并发、延迟发送、多条合并转发、自定义合并转发节点属性

```yaml
-
  regexp: "^一条很牛逼的合并转发$"
  reply:
    - "111"
    - 222
    -
      name: "不是QQ小冰"
      uin: 2854196306
      content:
        - "彳亍"
        - "笑死"
    - 彳亍
    - "[CQ:reply,qq=2854196306,text=000]333"
  scene: "all"
```

![corpus](https://github.com/Miuzarte/NothingBot/assets/66856838/15576647-c5ea-4948-8a13-947a7ac3ad81)

### 聊天相关

~~大部分功能都没什么用~~ 写着玩

**setu：**

`来(点|一点|几张|几份|.*张|.*份)?([Rr]18)?的?(.*)?的?[色瑟涩铯][图圖]|([Rr]18)?的?(.*)?的?[色瑟涩铯][图圖]来(点|一点|几张|几份|.*张|.*份)?`

点、几： rand[3,6]

x张、x份： 1~20内的阿拉伯数字、汉字大小写数字

可以使用 `&` 和 `|` 将多个关键词进行组合，`|` 的优先级永远高于 `&`，例如 `来张萝莉|少女&白丝|黑丝色图` 将会查找 tag 含有（“萝莉”或“少女”）且含有（“白丝”或“黑丝”）的 setu ~~（完全致敬cqps）~~

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

`@Bot{检查身体|运行状态}` 输出宿主机硬件信息、运行时长

### 哔哩哔哩相关

**动态推送：**

通过调用t.bilibili.com的新动态检测接口，3s拉取一次更新实现低延迟推送

**直播推送：**

通过建立直播间弹幕ws连接实现监听开播，[录播姬](https://github.com/BililiveRecorder/BililiveRecorder)同款

数据包解码[danmaku.go](danmaku.go)由[@moxcomic](https://github.com/moxcomic)编写

**链接解析：**

暂时只做了空间、动态、视频、专栏、音频、直播的（直链/短链）解析

**内容总结：**

在视频、文章、动态链接前面加上`总结一下`

无字幕内容会上传音频至必剪的文本转录接口识别字幕，使用的库：[BcutASR by @moxcomic](https://github.com/moxcomic/BcutAsr)

目前支持的后端：

- [ChatGLM2](https://github.com/THUDM/ChatGLM2-6B)：[api.py](https://github.com/THUDM/ChatGLM2-6B/blob/main/api.py)

- [百度千帆](https://console.bce.baidu.com/qianfan)："ERNIE_Bot", "ERNIE_Bot_turbo", "BLOOMZ_7B", Llama_2_7b", "Llama_2_13b", "Llama_2_70b"

```
GLM2在int4量化的情况下也足以用于总结内容
百度千帆申请会给20RMB免费额度（但是两千字符的限长是真的不够用）
```

**快捷搜索：**

`[Bb]搜(视频|番剧|影视|直播间|直播|主播|专栏|用户|相簿)[\s:：]?(.*)` 取决于类别，B站只会返回最多20或30条结果

### Pixiv

**kkp：**

`[看康k]{2}([Pp]|[Pp]站|[Pp][Ii][Dd]|[Pp][Ii][Xx][Ii][Vv])([0-9]+)`

### BertVITS2

[api.py](./apiForBertVITS2/api.py)

### 启动参数

**可选：**

- `-c` ：配置文件路径，默认"./config.yaml"

### TODO:

**重写轮子、哔哩哔哩视频解析上传**

[@moxcomic](https://github.com/moxcomic) :

![💩⛰️](https://github.com/Miuzarte/NothingBot/assets/66856838/98eb9a3e-c27c-4d08-8182-2332cf956198)

---

大学学费换来的最有价值的东西应该就是这个 `edu.cn` 结尾的教育邮箱

[![JetBrains](https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.svg)](https://www.jetbrains.com) [![GoLand](https://resources.jetbrains.com/storage/products/company/brand/logos/GoLand_icon.svg)](https://www.jetbrains.com/zh-cn/go/)
