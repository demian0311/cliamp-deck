#!/usr/bin/env python3
"""Outline and number the deck's panels (names go in the README legend) on a rendered capture.
annotate.py CAPTURE.ans OUT.png  — CAPTURE.png (from ansi2png.py) must sit beside the .ans.
Panels are found from the box corners in the capture; the status line is the last row.
Geometry must match ansi2png.py: CW×CH cells, pad px border."""
import re, sys
from PIL import Image, ImageDraw, ImageFont

CW, CH, PAD = 10, 20, 14
INK = (255, 106, 193)          # outline and badge: a pink no Omarchy theme uses for chrome
FONT = ImageFont.truetype('/usr/share/fonts/TTF/JetBrainsMonoNerdFont-Bold.ttf', 17)
# first word of a panel's title → label
NAMES = {'player': 'Player', 'eq': 'EQ', 'sources': 'Sources'}  # anything else is the visualizer

def boxes(rows):
    out = []
    for y, r in enumerate(rows):
        for x, ch in enumerate(r):
            if ch != '╭':
                continue
            x2 = r.index('╮', x)
            y2 = next(yy for yy in range(y + 1, len(rows)) if x < len(rows[yy]) and rows[yy][x] == '╰')
            title = re.sub(r'[─┤├╭╮]', ' ', r[x:x2]).split()
            name = NAMES.get(title[0] if title else '', 'Visualizer')
            out.append((name, x, y, x2, y2))
    return out

def px(x, y):
    return PAD + x * CW, PAD + y * CH

def main(ans, out):
    rows = [re.sub(r'\x1b\[[0-9;:]*m', '', l) for l in open(ans, encoding='utf-8').read().split('\n')]
    rows = [r for r in rows if r.strip()] and rows
    while rows and not rows[-1].strip():
        rows.pop()
    im = Image.open(ans.rsplit('.', 1)[0] + '.png').convert('RGB')
    d = ImageDraw.Draw(im)
    order = ['Player', 'EQ', 'Visualizer', 'Sources']
    found = sorted(boxes(rows), key=lambda b: order.index(b[0]))
    w = max(len(r) for r in rows)
    found.append(('Status line', 0, len(rows) - 1, w - 1, len(rows) - 1))
    for n, (name, x1, y1, x2, y2) in enumerate(found, 1):
        (a, b), (c, e) = px(x1, y1), px(x2 + 1, y2 + 1)
        # inset, so panels that share an edge keep separate outlines
        d.rounded_rectangle([a + 1, b + 1, c - 2, e - 2], radius=8, outline=INK, width=3)
        # a numbered disc on the box's top-left corner, where no title or pager ever sits;
        # the status line has no border, so its disc goes left of the connection dot
        cx, cy = (a + 14, b + 2) if name != "Status line" else (c - 150, (b + e) / 2)
        d.ellipse([cx - 14, cy - 14, cx + 14, cy + 14], fill=INK, outline=(20, 20, 24), width=2)
        d.text((cx, cy + 1), str(n), font=FONT, fill=(20, 20, 24), anchor='mm')
    im.save(out, optimize=True)

if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])
