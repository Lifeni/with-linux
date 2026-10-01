#!/usr/bin/env python3
"""生成 README 顶部的海报（assets/banner-{light,dark}.svg）。

海报完全复用网页的资源：底纹就是 web/dots.svg 那份点阵（只把 viewBox 下移，
露出最下面 560px），图标就是 assets/icon.svg（与 web/favicon.svg 同一枚），
颜色取网页的背景色与点阵色。
文字用字体轮廓转成 path，不依赖浏览端字体，任何地方渲染都一致。

依赖：python3 + fontTools（pip install fonttools）
用法：python3 scripts/gen-banner.py
"""

import os
import re
import xml.etree.ElementTree as ET

from fontTools.misc.transform import Transform
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SVG_NS = "http://www.w3.org/2000/svg"

# 海报几何：直接切 web/dots.svg 的下面这段（坐标系沿用点阵的坐标系）。
VIEW_BOX = (0, 280, 1680, 560)

# 内容排布（都在上面的坐标系里）。字体大小 = DejaVu 的单位数 × scale，upem 是 2048。
LOGO_X, LOGO_Y, LOGO_SIZE = 88, 651, 112
TEXT_X = 236
TITLE_BASELINE, TITLE_SIZE = 710, 76
SUB_BASELINE, SUB_SIZE = 752, 22
TITLE = "With Linux"
SUBTITLE = "Terminal toolbox for Linux · arm64 / amd64"

FONT_FILES = {
    "bold": "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf",
    "regular": "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
}

THEMES = [
    {
        "name": "dark",
        "bg": "#0e1114",
        "dot": "#8fb2d6",
        "title": "#eceff3",
        "subtitle": "#a7b0bb",
    },
    {
        "name": "light",
        "bg": "#fafaf8",
        "dot": "#5f87af",
        "title": "#14161a",
        "subtitle": "#4a515c",
    },
]


def text_path(weight, text, size):
    """把字符串转成一条 path；坐标是 font units，用 scale(s, -s) 落到 SVG 上。"""
    font = TTFont(FONT_FILES[weight])
    upem = font["head"].unitsPerEm
    cmap = font.getBestCmap()
    glyphs = font.getGlyphSet()
    hmtx = font["hmtx"]
    parts, x = [], 0.0
    for ch in text:
        name = cmap.get(ord(ch))
        if name is None:
            raise SystemExit(f"字体里没有字形：{ch!r}")
        pen = SVGPathPen(glyphs)
        glyphs[name].draw(TransformPen(pen, Transform(1, 0, 0, 1, x, 0)))
        if pen.getCommands():
            parts.append(pen.getCommands())
        x += hmtx[name][0]
    return " ".join(parts), size / upem


def read_field():
    """从 web/dots.svg 读出点阵：每组的不透明度 + 组内的圆。"""
    tree = ET.parse(os.path.join(ROOT, "web/dots.svg"))
    groups = []
    for g in tree.getroot().findall(f"{{{SVG_NS}}}g"):
        circles = [
            (c.get("cx"), c.get("cy"), c.get("r"))
            for c in g.findall(f"{{{SVG_NS}}}circle")
        ]
        groups.append((g.get("fill-opacity"), circles))
    return groups


def read_logo():
    """从 assets/icon.svg 读出图标本体，去掉投影滤镜（海报上不需要，也更好兼容）。"""
    tree = ET.parse(os.path.join(ROOT, "assets/icon.svg"))
    root = tree.getroot()
    ET.register_namespace("", SVG_NS)
    parts = []
    for child in root:
        tag = child.tag.split("}")[-1]
        if tag == "defs":
            keep = [
                e
                for e in child
                if e.tag.split("}")[-1] == "linearGradient"
            ]
            if keep:
                d = ET.Element(f"{{{SVG_NS}}}defs")
                d.extend(keep)
                parts.append(ET.tostring(d, encoding="unicode"))
        elif tag == "g":
            # 丢掉 filter 属性，只留里面的图形
            g = ET.Element(f"{{{SVG_NS}}}g")
            g.extend(list(child))
            parts.append(ET.tostring(g, encoding="unicode"))
        else:
            parts.append(ET.tostring(child, encoding="unicode"))
    return parts


def build_svg(theme, field, logo, title, subtitle):
    x, y, w, h = VIEW_BOX
    out = [
        f'<svg xmlns="{SVG_NS}" viewBox="{x} {y} {w} {h}" width="{w}" height="{h}" '
        f'role="img" aria-label="{TITLE}">',
        f'  <rect x="{x}" y="{y}" width="{w}" height="{h}" fill="{theme["bg"]}"/>',
        f'  <g fill="{theme["dot"]}">',
    ]
    for opacity, circles in field:
        out.append(f'    <g fill-opacity="{opacity}">')
        for cx, cy, r in circles:
            out.append(f'      <circle cx="{cx}" cy="{cy}" r="{r}"/>')
        out.append("    </g>")
    out.append("  </g>")

    scale = LOGO_SIZE / 1024
    out.append(f'  <g transform="translate({LOGO_X} {LOGO_Y}) scale({scale:g})">')
    out.extend("    " + s for s in logo)
    out.append("  </g>")

    out.append(
        f'  <path fill="{theme["title"]}" '
        f'transform="translate({TEXT_X} {TITLE_BASELINE}) scale({title[1]:g} {-title[1]:g})" '
        f'd="{title[0]}"/>'
    )
    out.append(
        f'  <path fill="{theme["subtitle"]}" '
        f'transform="translate({TEXT_X} {SUB_BASELINE}) scale({subtitle[1]:g} {-subtitle[1]:g})" '
        f'd="{subtitle[0]}"/>'
    )
    out.append("</svg>\n")
    return "\n".join(out)


def main():
    field = read_field()
    logo = read_logo()
    title = text_path("bold", TITLE, TITLE_SIZE)
    subtitle = text_path("regular", SUBTITLE, SUB_SIZE)

    out_dir = os.path.join(ROOT, "assets")
    os.makedirs(out_dir, exist_ok=True)
    for theme in THEMES:
        svg = build_svg(theme, field, logo, title, subtitle)
        path = os.path.join(out_dir, f'banner-{theme["name"]}.svg')
        with open(path, "w", encoding="utf-8") as f:
            f.write(svg)
        print(f'写出 {os.path.relpath(path, ROOT)}：{len(svg) / 1024:.1f} KB')


if __name__ == "__main__":
    main()
