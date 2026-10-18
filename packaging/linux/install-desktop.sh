#!/bin/sh
# Add this unpacked game to the current user's application menu.
set -eu

app_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ ! -x "$app_dir/goldminer" ] || [ ! -f "$app_dir/goldminer.png" ]; then
    printf '请先完整解压安装包，再运行此脚本。\n' >&2
    exit 1
fi

applications="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
mkdir -p "$applications"

# Exec has both Desktop Entry string escapes and command-line quote escapes.
exec_path=$(printf '%s' "$app_dir/goldminer" |
    sed 's/\\/\\\\/g; s/"/\\"/g; s/`/\\`/g; s/\$/\\$/g; s/%/%%/g' |
    sed 's/\\/\\\\/g')
icon_path=$(printf '%s' "$app_dir/goldminer.png" | sed 's/\\/\\\\/g')

# Keep the executable path as an argument so GIO can handle literal %% in it.
printf '[Desktop Entry]\nType=Application\nName=黄金矿工\nExec=env "%s"\nIcon=%s\nTerminal=false\nCategories=Game;\nStartupWMClass=GoldMiner\n' \
    "$exec_path" "$icon_path" > "$applications/goldminer.desktop"

printf '已添加“黄金矿工”到应用菜单。请保留当前安装目录。\n'
