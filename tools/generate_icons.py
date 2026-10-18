#!/usr/bin/env python3
"""Export platform icon formats from the generated transparent master artwork."""

from pathlib import Path

from PIL import Image


icons = Path(__file__).resolve().parent.parent / "assets" / "icons"
with Image.open(icons / "goldminer-source.png") as source:
    master = source.convert("RGBA")
    master.resize((512, 512), Image.Resampling.LANCZOS).save(icons / "goldminer.png")
    master.save(
        icons / "goldminer.ico",
        sizes=[(size, size) for size in (16, 24, 32, 48, 64, 128, 256)],
    )
    # Supply Lanczos-resampled PNG frames for the ICNS container.
    frames = [
        master.resize((size, size), Image.Resampling.LANCZOS)
        for size in (32, 64, 128, 256, 512)
    ]
    master.resize((1024, 1024), Image.Resampling.LANCZOS).save(
        icons / "goldminer.icns", append_images=frames
    )

print("Generated goldminer.png, goldminer.ico and goldminer.icns")
