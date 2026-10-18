# 黄金矿工 Go 桌面版

这个项目用 Go 运行所选《黄金矿工》SWF 的原始时间轴、关卡、指令与素材，并加入中文界面、暂停、自动存档与双人合作模式。不需要 Flash Player、Ruffle 或浏览器。运行时不读取 SWF 文件，游戏数据与资源通过 `go:embed` 放进可执行程序。

为了保留原作的逐帧动作和特殊规则，实现方式是 **Go 编写的专用 Flash 5 时间轴与 AVM1 指令执行器**，而不是把 ActionScript 逐行改写为 Go 游戏逻辑。可以查看 `internal/flash` 的 Go 实现，以及 `reference/actionscript` 中的反编译脚本。

**目前是可运行的忠实移植，尚未证明达到像素与全部交互逐帧完全一致。** 原素材、布局、关卡选择和计分指令已保留；图形由原矢量导出后在 Go 中绘制，抗锯齿和部分渐变与参考播放器仍有细微差异。对照图放在 `verification/`，便于直接检查。

## 运行

从 [GitHub Releases](https://github.com/Yi-Jiao-Chan/gold-miner-go/releases/latest) 下载对应平台的安装包，程序自带资源，无需安装 Go 或 Flash。

| 平台 | 安装包 | 运行 |
| --- | --- | --- |
| macOS Apple 芯片 | `gold-miner-macos-arm64-v2.0.6.zip` | macOS 12+，解压后双击 `黄金矿工.app` |
| Windows x64 | `gold-miner-windows-amd64-v2.0.6.zip` | 解压后双击 `goldminer.exe` |
| Linux x64 | `gold-miner-linux-amd64-v2.0.6.zip` | glibc 2.34+，解压后运行 `./goldminer`，需要桌面图形和音频环境 |

Mac 应用未经过 Apple Developer ID 签名或公证。被系统阻止时，按安装包内说明允许打开。Linux 版本已实际运行；Windows 与 Mac 版本已编译及校验，尚未做对应系统的实机运行测试。

程序和窗口使用老矿工与金块图标：Windows 图标已嵌入 exe，Mac 图标包含在 `.app` 中。Linux 解压后可运行 `./install-desktop.sh` 添加带图标的应用菜单入口；添加后请保留安装目录。

从源码启动，安装 Go 1.25 或更高版本，然后在本目录运行：

```sh
go run ./cmd/goldminer
```

在没有正常 Git 元数据的目录下可使用：

```sh
go run -buildvcs=false ./cmd/goldminer
```

Windows 和 macOS 的 Ebitengine 桌面构建按其 [官方安装说明](https://ebitengine.org/en/documents/install.html)准备环境。Linux 编译需要 GCC 及 X11、OpenGL、ALSA 开发库，例如 Debian/Ubuntu：

```sh
sudo apt-get install gcc libx11-dev libxrandr-dev libxinerama-dev libxcursor-dev libxi-dev libxxf86vm-dev libgl1-mesa-dev libasound2-dev
```

## 操作

首页可以选择“单人开始”“双人合作”或“继续存档”。单人用 ↓ 放下抓钩、↑ 使用炸药、←/→ 提前收回；双人左侧用 S 放下抓钩、W 使用炸药，右侧用 ↓ 放下抓钩、↑ 使用炸药、←/→ 提前收回。双矿工间距参照双人版本调整为 70 个舞台像素，旧存档加载时同步调整位置，保留运动状态。双人每关完整计时 40 秒，同时挖掘同一关的矿物，分别累计抓取分数，以两人分数之和判定过关，目标金额与原作一致。炸药和商店加成共享，购买道具时两人各承担一半费用，从各自分数中扣除；奇数价格各扣到 0.5 分，个人分数可为负数，购买仍使用合计金额。关卡内的个人分数以绿色直接显示在对应矿工下方，无底色和位置文字；商店和游戏结束界面也显示两人的当前分数。鼠标在商店选购物品，再点击“下一关”。初始窗口为 1100×800，舞台坐标保持 550×400；按窗口及 Retina 实际像素直接绘制，放大不会再经过 550×400 的低分辨率画面。输入每秒处理 120 次，原游戏时间轴保持每秒 18 帧。改变窗口大小后鼠标坐标按相同的缩放和留边换算。

## 暂停与存档

关卡内点击右上方“暂停”或按 Esc，整关冻结，包括矿工、矿物、动画与倒计时。点击“恢复游戏”后显示 3、2、1，完整倒数 3 秒后继续；倒数时也可以点“保持暂停”。

每次新游戏创建一个存档，此后每关结束自动更新同一个存档。购买商品、进入下一关、返回首页以及关闭窗口时也保存当前状态。重新打开后从“继续存档”选择未结束的一局；关卡、两位矿工的抓钩、随机状态、金钱、个人分数、道具和商店购买状态都会恢复。旧双人存档缺少个人分数时，续玩会将当前总分均分给两人一次，此后分别累计并保存。加载进行中的关卡后先倒数完整 3 秒，期间画面冻结；商店存档直接进入商店。

存档只保留未结束的游戏，最多 100 个。满额时需要先在存档列表中确认删除一个，再开启新局。游戏结束后自动从存档移除，仅将模式、达到的关卡、金额、目标和结束时间写入“历史记录”，不保留可恢复的关卡数据。历史记录不占存档名额，可单独查看和删除。旧版的已结束存档会自动迁移到历史记录；损坏文件会被跳过并显示提示，原文件保留。

默认保存位置：macOS 为 `~/Library/Application Support/GoldMiner/saves`；Linux 为 `${XDG_CONFIG_HOME:-~/.config}/GoldMiner/saves`。存档不放在 `.app` 中，替换应用不会清除存档和历史记录。历史摘要位于该目录的 `history/` 子目录。`--save-dir` 可指定另一个目录用于验证或便携运行。

## 保留的原始内容

| 内容 | 来源与行为 |
| --- | --- |
| 30 种布局 | 原时间轴 `L1_1` 至 `L10_3`，保留坐标、变换、对象和深度顺序 |
| 关卡选择 | 每组从三种布局中随机选择；第 10 组后回到第 4 组，显示关数继续增加 |
| 目标分数 | 保留原累计目标及递增公式，商店花费会减少当前金额 |
| 抓钩与回收 | 原时间轴控制轨迹和物品重量对应的回收帧步长；双人各有独立抓钩 |
| 计分时机 | 抓取后原 bonus 时间轴第 32 帧才入账 |
| 商店与袋子 | 原价格、概率、炸药、力量、幸运草、石头与钻石加成指令 |
| 动画 | 原角色、移动动物、抓钩、爆炸与奖励时间轴 |
| 字体与音效 | 提取的 9 个字体与 11 个音效，保留音效截取、循环和音量设置 |

前十关的累计目标为：

| 显示关数 | 目标 |
| --- | ---: |
| 1 | $650 |
| 2 | $1,195 |
| 3 | $2,010 |
| 4 | $3,095 |
| 5 | $4,450 |
| 6 | $6,075 |
| 7 | $7,970 |
| 8 | $10,135 |
| 9 | $12,570 |
| 10 | $15,275 |

此后每关目标再增加 $2,705。随机数使用 Go 的均匀随机数生成器；`--seed` 可以复现 Go 版本的同一次运行，不代表与 Adobe Flash 的随机序列相同。

## 验证与限制

在有显示服务的桌面运行测试与检查（渲染测试需要图形环境；Linux 无显示环境可使用 Xvfb）：

```sh
go test ./...
go test -race ./internal/flash ./internal/audiofx
go vet ./...
```

已验证原始开局与累计目标、全部 30 种布局的对象初始化、原按键与鼠标事件、倒计时、商店切换和购买、碰撞后的原时间轴计分。修复了商店商品时间轴重复初始化、闪烁及价格反复变化的问题，五种商品均经过连续鼠标交互和重复购买检查。本次程序图标及三平台构建的验证见 `verification/github-release-v2.0.6-report.json`；计分显示调整的验证见 `verification/github-release-v2.0.5-report.json`；双人独立计分、购物均摊和旧档续玩的验证见 `verification/github-release-v2.0.4-report.json`；此前中文、暂停、双人、存档的验证见 `verification/github-release-v2.0.3-report.json`，早期原作对照见 `verification/report.json`。已在 Linux 原生窗口用实际键鼠试玩过关、购物、玩家二炸药和退出重启。Apple 芯片版已交叉编译并校验应用包，尚未在 Mac 实机测试。

截图辅助命令不改变正常游戏菜单：

```sh
go run ./cmd/goldminer --seed 1 --layout L1_1 --snapshot level.png
go run ./cmd/goldminer --seed 1 --frame 10 --snapshot menu.png
```

`--layout` 可选 `L1_1` 至 `L10_3`，会用原目标递增脚本设置对应关数；`--ticks` 在截图前推进指定帧数。截图分辨率跟随窗口和屏幕缩放；`--scale 1` 可在普通屏幕导出原始 550×400 对照图。Linux 的无显示环境需要 Xvfb。

存档的临时文件在各系统上均先同步到磁盘，再替换正式文件。Unix 系统会同步目录；Windows 跳过目录同步，因为 [FlushFileBuffers 要求可写句柄](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers)，对只读目录句柄执行该操作会失败。

当前专用执行器支持这个文件实际用到的 35 种指令，不是通用 SWF 播放器。旧网站的“提交分数”和推广按钮会把 URL 记录到 `Runtime.BlockedURLs`，不会向已经过时的服务器发起请求。音效已按原元数据处理，但尚未与 Adobe Flash 输出做波形对照。参考画面使用 Ruffle 0.7.1，因此也不构成与 Adobe Flash 每一像素完全相同的认证。

## 文件与复现

- `cmd/goldminer/main.go`：桌面窗口、输入、音效和截图入口。
- `internal/flash/`：Go 时间轴、显示树、碰撞和 AVM1 指令执行。
- `internal/render/`：原素材与嵌入字体的 Go 绘制。
- `internal/audiofx/`：原 SOUNDINFO 的音效截取、循环和通道音量。
- `internal/savegame/`：原子存档、100 个活动存档限制、历史摘要迁移、删除与损坏隔离。
- `internal/flash/checkpoint.go`：完整显示树、变量与随机状态的保存 / 恢复。
- `internal/flash/coop.go`：双矿工、独立输入、共享物品与 40 秒计时。
- `internal/i18n/` 与 `assets/chinese.json`：中文文本与原文本映射。
- `assets/movie.json`：原始时间轴、脚本指令、关卡与素材清单。
- `reference/gold-miner.swf`：用于审计的原文件；运行不依赖它。
- `reference/actionscript/`：反编译脚本，保留原文件路径。
- `tools/extract.py`：从 FFDec 导出结果生成资源数据的脚本。
- `assets/icons/` 与 `tools/generate_icons.py`：图标原图及 PNG、ICO、ICNS 格式导出；完整生成提示见该目录的 README。
- `tools/generate_windows_icon.sh`：用固定版本工具重新生成 Windows amd64 图标资源；已提交的 `.syso` 会随 Go 构建自动链接。
- `packaging/linux/install-desktop.sh`：Linux 安装包中的应用菜单快捷方式安装脚本。
- `verification/`：Go 和原 SWF 的同布局截图、对照和检查报告。

使用 [JPEXS FFDec 26.3.0](https://github.com/jindrapetrik/jpexs-decompiler) 导出 XML、脚本、PNG、SVG、字体与 WAV 后，运行提取工具；该工具需要 Pillow、CairoSVG 和 fontTools。中文采用随程序嵌入的 Noto Sans SC 字体，按 SIL Open Font License 1.1 分发，许可证位于 `assets/fonts/OFL.txt`。小图形采用 8 倍分辨率导出，大图形和静态文字采用 4 倍（超大预加载纯色图形仍为 2 倍）。动态文字按当前绘制尺寸栅格化，直接绘制到屏幕实际分辨率。字体的 SFNT 容器表经过格式修正，保留提取的字形轮廓和度量。

原文件来自 [Flash 保存计划的黄金矿工存档](https://flash.homes/flash/1GYM-20M5-TTD3)，大小 298,548 字节，SHA-256：

```text
0c3d410285d69a36022f652b0c28e5c5f6d56b6112d6c77500c9b09387122eeb
```

原作、提取图形、字体和声音的权利归各自权利人；此移植项目未获得新的素材授权。
