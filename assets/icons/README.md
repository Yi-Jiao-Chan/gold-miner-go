# 程序图标

`goldminer-source.png` 是使用内置 imagegen 工具生成的透明原图，参考原作矿工头像 `assets/shapes/251.png` 和游戏截图 `verification/release-2.0.5-coop-scores.png`。没有使用 CLI/API 回退。原作人物和素材的权利归原权利人。

运行 `python3 tools/generate_icons.py` 导出窗口与 Linux 使用的 `goldminer.png`、Windows 多尺寸 `goldminer.ico` 和 Mac 的 `goldminer.icns`。Pillow 已包含在 `tools/requirements.txt` 中。运行 `sh tools/generate_windows_icon.sh` 后，Windows amd64 的后续 Go 构建会自动嵌入图标资源。

Mac 打包时将 `goldminer.icns` 放入 `黄金矿工.app/Contents/Resources/`，并设置 `CFBundleIconFile` 为 `goldminer.icns`。Linux 包同时提供 PNG 和 `install-desktop.sh`；用户运行脚本后，应用菜单入口会使用此图标。

完整生成提示词：

> Use case: compositing. Create a production application icon for this Gold Miner desktop game by transforming the supplied character portrait into a compact, recognizable icon. Image 1 is the edit target and character identity reference: preserve this specific elderly cartoon miner's large peach-colored nose, blue eyes, full light gray beard, gray hair, thick outlines and simple cel-shaded Flash-game style. Image 2 is a supporting reference for his original broad pale straw hat and the game's warm gold/brown palette; do not reproduce the screenshot, UI, numerals, or scenery. Show a close-up bust of that same miner wearing his straw hat, with one clear shiny gold nugget at chest height. Use a simple warm dark-brown and golden rounded-square backing tile, with real transparent alpha outside its rounded corners. The miner's face and hat should dominate the icon and remain recognizable at 32px. Center the full design with comfortable outer margins so nothing touches or is clipped by the canvas. Square 1024x1024 master, crisp clean cartoon silhouettes, strong readable contrast, limited large color areas. Friendly classic-game character, not photorealistic and not a new modern construction-worker design. No letters, words, numbers, badges, watermark, interface, extra people, or small decorative clutter. The output is one finished app icon, not a contact sheet or mockup.
