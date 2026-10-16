#!/usr/bin/env python3
"""生成《开发指南》第三节的开发流程图（回行 / boustrophedon 布局）。

mermaid 的自动布局做不到"从阶段二框底正中引出向下箭头、落到正下方阶段三框顶"——
跨层指向具体节点的连线会让整层塌成竖列，所以这里用 Pillow 直接画一张等价的静态图。

用法：
    python3 scripts/draw_dev_flow.py            # 输出 docs/assets/dev-flow.png
"""
from __future__ import annotations

import os
from PIL import Image, ImageDraw, ImageFont

SCALE = 2  # 2x 输出，保证在高分屏和缩放下依然清晰
OUT = os.path.join("docs", "assets", "dev-flow.png")

FONT_PATH = "/System/Library/Fonts/Hiragino Sans GB.ttc"
FONT_BOLD_PATH = "/System/Library/Fonts/Hiragino Sans GB.ttc"

INK = "#0F172A"
LINE = "#475569"
LABEL_BG = "#E2E8F0"

STYLES = {
    "startend": ("#EDE9FE", "#8B5CF6"),
    "process": ("#E0F2FE", "#3B82F6"),
    "decision": ("#FEF9C3", "#F59E0B"),
    "result": ("#DCFCE7", "#22C55E"),
}


def font(size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(FONT_PATH, size * SCALE, index=0)


F_TITLE = font(32)
F_SUB = font(26)


class Canvas:
    def __init__(self, width: int, height: int) -> None:
        self.img = Image.new("RGB", (width * SCALE, height * SCALE), "white")
        self.d = ImageDraw.Draw(self.img)

    # ---------- 基本图形 ----------
    def box(self, x, y, w, h, lines, kind="process", rounded=False):
        fill, stroke = STYLES[kind]
        self.d.rounded_rectangle(
            [x * SCALE, y * SCALE, (x + w) * SCALE, (y + h) * SCALE],
            radius=(h // 2 if rounded else 14) * SCALE,
            fill=fill, outline=stroke, width=2 * SCALE,
        )
        self._text_block(x + w / 2, y + h / 2, lines)
        return (x, y, w, h)

    def diamond(self, cx, cy, hw, hh, lines, kind="decision"):
        fill, stroke = STYLES[kind]
        pts = [(cx, cy - hh), (cx + hw, cy), (cx, cy + hh), (cx - hw, cy)]
        self.d.polygon([(px * SCALE, py * SCALE) for px, py in pts],
                       fill=fill, outline=stroke)
        self.d.line([(px * SCALE, py * SCALE) for px, py in pts + [pts[0]]],
                    fill=stroke, width=2 * SCALE, joint="curve")
        self._text_block(cx, cy, lines)
        return pts

    def _text_block(self, cx, cy, lines):
        gap = 6 * SCALE
        heights = []
        for i, text in enumerate(lines):
            f = F_TITLE if i == 0 else F_SUB
            b = self.d.textbbox((0, 0), text, font=f)
            heights.append((b[3] - b[1], f))
        total = sum(h for h, _ in heights) + gap * (len(lines) - 1)
        y = cy * SCALE - total / 2
        for (h, f), text in zip(heights, lines):
            self.d.text((cx * SCALE, y + h / 2), text, font=f, fill=INK, anchor="mm")
            y += h + gap

    # ---------- 连线 ----------
    def arrow(self, points, label=None, label_offset=(0, -18), dashed=False):
        pts = [(px * SCALE, py * SCALE) for px, py in points]
        self.d.line(pts, fill=LINE, width=2 * SCALE, joint="curve")
        self._arrow_head(pts[-2], pts[-1])
        if label:
            mx = (pts[-2][0] + pts[-1][0]) / 2 + label_offset[0] * SCALE
            my = (pts[-2][1] + pts[-1][1]) / 2 + label_offset[1] * SCALE
            self._label(mx, my, label)

    def _arrow_head(self, prev, tip, size=13):
        (x0, y0), (x1, y1) = prev, tip
        dx, dy = x1 - x0, y1 - y0
        n = (dx * dx + dy * dy) ** 0.5 or 1.0
        ux, uy = dx / n, dy / n
        px, py = -uy, ux
        s = size * SCALE
        self.d.polygon([
            (x1, y1),
            (x1 - ux * s + px * s * 0.55, y1 - uy * s + py * s * 0.55),
            (x1 - ux * s - px * s * 0.55, y1 - uy * s - py * s * 0.55),
        ], fill=LINE)

    def _label(self, cx, cy, text):
        f = F_SUB
        lines = text.split("|")
        sizes = []
        for line in lines:
            b = self.d.textbbox((0, 0), line, font=f)
            sizes.append((b[2] - b[0], b[3] - b[1]))
        w = max(s[0] for s in sizes)
        lh = max(s[1] for s in sizes) + 4 * SCALE
        h = lh * len(lines)
        pad = 8 * SCALE
        self.d.rounded_rectangle(
            [cx - w / 2 - pad, cy - h / 2 - pad * 0.6,
             cx + w / 2 + pad, cy + h / 2 + pad * 0.6],
            radius=6 * SCALE, fill=LABEL_BG)
        y = cy - h / 2
        for line in lines:
            self.d.text((cx, y + lh / 2), line, font=f, fill="#334155", anchor="mm")
            y += lh


def main() -> None:
    # ---- 三列网格：第 1 层向右，第 2 层向左，第 3、4 层沿左列向下 ----
    c1, w1 = 60, 380
    c2, w2 = 620, 380
    c3, w3 = 1050, 440
    r1, hbox = 100, 120
    r2 = 450
    r3 = 800
    W, H = 1550, 1480

    cv = Canvas(W, H)

    # ---------- 第 1 层（自左向右）----------
    cv.box(c1, r1, w1, hbox, ["原型愿景", "分布式文件存储", "+ 多语言 SDK"], "startend", rounded=True)
    cv.box(c2, r1, w2, hbox, ["阶段一 架构原型", "三层架构 · 共识 · 分片"])
    cv.box(c3, r1, w3, hbox, ["阶段二 v0.1 文件服务重构", "砍范围 · 文件闭环 · 认证"])
    cv.arrow([(c1 + w1, r1 + hbox / 2), (c2, r1 + hbox / 2)], label="砍范围重做")
    cv.arrow([(c2 + w2, r1 + hbox / 2), (c3, r1 + hbox / 2)])

    # ---------- 折返：阶段二框底正中 → 阶段三框顶 ----------
    cv.arrow([(c3 + w3 / 2, r1 + hbox), (c3 + w3 / 2, r2)])

    # ---------- 第 2 层（自右向左）----------
    cv.box(c3, r2, w3, hbox, ["阶段三 v1.0 生产化上线", "原子落盘 · 沙箱部署 · 验收"])
    cv.box(c2, r2, w2, hbox, ["阶段四 运维与可靠性扩展", "xionctl · 续传 · 回收站"])
    cv.box(c1, r2, w1, hbox, ["阶段五 文档与品牌", "Logo · README 导航"])
    cv.arrow([(c3, r2 + hbox / 2), (c2 + w2, r2 + hbox / 2)])
    cv.arrow([(c2, r2 + hbox / 2), (c1 + w1, r2 + hbox / 2)])

    # ---------- 折返：阶段五框底正中 → 阶段六框顶 ----------
    cv.arrow([(c1 + w1 / 2, r2 + hbox), (c1 + w1 / 2, r3)])

    # ---------- 第 3 层：阶段六（左列）----------
    cv.box(c1, r3, w1, hbox, ["阶段六 v1.1", "加固与自动发布", "崩溃自愈 · 配额索引"])

    # ---------- 折返：阶段六框底正中 → 回归验证菱形顶 ----------
    dcx, dcy, dhw, dhh = c1 + w1 / 2, 1145, 208, 158
    cv.arrow([(dcx, r3 + hbox), (dcx, dcy - dhh)])

    # ---------- 第 4 层：回归验证与交付 / 缺陷回路 ----------
    cv.diamond(dcx, dcy, dhw, dhh, ["回归验证通过吗？", "test · race · vet · pytest"])

    bx, bw, bh = 700, 560, 110
    cv.box(bx, dcy - 110, bw, bh, ["单节点生产文件服务", "+ 实验性分布式扩展"], "result", rounded=True)
    cv.arrow([(dcx + dhw * 0.5, dcy - dhh * 0.5), (bx, dcy - 55)],
             label="通过", label_offset=(0, -20))

    cv.box(bx, dcy + 70, bw, bh, ["定位缺陷并补测试用例", "往返 · 故障注入 · 状态迁移"])
    cv.arrow([(dcx + dhw * 0.5, dcy + dhh * 0.5), (bx, dcy + 125)],
             label="不通过", label_offset=(0, -20))

    # 修复后重跑：从缺陷框下方绕回菱形底顶点
    cv.arrow([(bx + bw / 2, dcy + 180), (bx + bw / 2, dcy + 260),
              (dcx, dcy + 260), (dcx, dcy + dhh)],
             label="修复后重跑", label_offset=(120, 22))

    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    cv.img.save(OUT)
    print(f"{OUT}: {os.path.getsize(OUT)} bytes, {cv.img.size[0]}x{cv.img.size[1]}")


if __name__ == "__main__":
    main()
