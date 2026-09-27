#!/usr/bin/env python3
"""Render a `tmux capture-pane -e` dump to PNG with the Omarchy theme's terminal palette.
Half-blocks, braille and box drawing are drawn as geometry so cells tile without gaps."""
import re, sys, os
from PIL import Image, ImageDraw, ImageFont

CW, CH = 10, 20
FONT = ImageFont.truetype('/usr/share/fonts/TTF/JetBrainsMonoNerdFont-Regular.ttf', 16)
BOLD = ImageFont.truetype('/usr/share/fonts/TTF/JetBrainsMonoNerdFont-Bold.ttf', 16)

def palette():
    """The terminal palette of THEME_DIR (an Omarchy theme directory), or of the applied theme.
    A theme that has not been applied has no generated foot.ini, so its colors.toml is mapped the
    way Omarchy's foot.ini.tpl does."""
    d = os.environ.get('THEME_DIR') or os.path.expanduser('~/.local/state/omarchy/current/theme')
    if not os.path.exists(os.path.join(d, 'foot.ini')):
        c = {}
        for line in open(os.path.join(d, 'colors.toml')):
            k, _, v = line.partition('=')
            v = v.strip().strip('"\'').lstrip('#')
            if re.fullmatch(r'[0-9a-fA-F]{6}', v):
                c[k.strip()] = tuple(int(v[i:i+2], 16) for i in (0, 2, 4))
        g = lambda *ks: next(c[k] for k in ks if k in c)
        reg = [g('background'), g('red'), g('green'), g('yellow'), g('blue'), g('purple', 'magenta'), g('cyan'), g('foreground')]
        bri = [g('muted', 'foreground'), g('bright_red', 'red'), g('bright_green', 'green'), g('bright_yellow', 'yellow'),
               g('bright_blue', 'blue'), g('bright_magenta', 'magenta'), g('bright_cyan', 'cyan'), g('bright_foreground')]
        return reg + bri, g('foreground'), g('background')
    p = {}
    for line in open(os.path.join(d, 'foot.ini')):
        k, _, v = line.strip().partition('=')
        v = v.split()[0] if v else ''
        if re.fullmatch(r'[0-9a-fA-F]{6}', v):
            p[k] = tuple(int(v[i:i+2], 16) for i in (0, 2, 4))
    pal = [p[f'regular{i}'] for i in range(8)] + [p[f'bright{i}'] for i in range(8)]
    return pal, p['foreground'], p['background']

PAL, FG, BG = palette()

def xterm256(n):
    if n < 16: return PAL[n]
    if n < 232:
        n -= 16; lv = [0, 95, 135, 175, 215, 255]
        return (lv[n // 36], lv[n // 6 % 6], lv[n % 6])
    g = 8 + (n - 232) * 10; return (g, g, g)

def parse(path):
    rows = []
    st = {'fg': None, 'bg': None, 'bold': False, 'rev': False}
    for line in open(path, encoding='utf-8').read().split('\n'):
        cells = []
        for tok in re.split(r'(\x1b\[[0-9;:]*m)', line):
            if tok.startswith('\x1b['):
                ps = [int(x) if x else 0 for x in re.split('[;:]', tok[2:-1])] or [0]
                i = 0
                while i < len(ps):
                    c = ps[i]
                    if c == 0: st.update(fg=None, bg=None, bold=False, rev=False)
                    elif c == 1: st['bold'] = True
                    elif c == 22: st['bold'] = False
                    elif c == 7: st['rev'] = True
                    elif c == 27: st['rev'] = False
                    elif 30 <= c <= 37: st['fg'] = PAL[c - 30]
                    elif 90 <= c <= 97: st['fg'] = PAL[c - 82]
                    elif 40 <= c <= 47: st['bg'] = PAL[c - 40]
                    elif 100 <= c <= 107: st['bg'] = PAL[c - 92]
                    elif c == 39: st['fg'] = None
                    elif c == 49: st['bg'] = None
                    elif c in (38, 48):
                        key = 'fg' if c == 38 else 'bg'
                        if ps[i+1] == 5: st[key] = xterm256(ps[i+2]); i += 2
                        elif ps[i+1] == 2: st[key] = tuple(ps[i+2:i+5]); i += 4
                    i += 1
            else:
                for ch in tok:
                    fg, bg = st['fg'] or FG, st['bg'] or BG
                    if st['rev']: fg, bg = bg, fg
                    cells.append((ch, fg, bg, st['bold']))
        rows.append(cells)
    while rows and not rows[-1]: rows.pop()
    return rows

BOX = {'─': 'lr', '━': 'LR', '│': 'ud', '┃': 'UD', '╭': 'rd', '╮': 'ld', '╰': 'ru', '╯': 'lu', '┤': 'udl', '├': 'udr'}

def cell(d, x, y, ch, fg, bg, bold):
    X, Y = x * CW, y * CH
    d.rectangle([X, Y, X + CW - 1, Y + CH - 1], fill=bg)
    if ch == ' ': return
    if ch == '▀':
        d.rectangle([X, Y + CH // 2, X + CW - 1, Y + CH - 1], fill=bg)
        d.rectangle([X, Y, X + CW - 1, Y + CH // 2 - 1], fill=fg); return
    o = ord(ch)
    if 0x2800 <= o <= 0x28FF:
        bits = o - 0x2800
        dots = [(0, 0, 0x01), (0, 1, 0x02), (0, 2, 0x04), (1, 0, 0x08), (1, 1, 0x10), (1, 2, 0x20), (0, 3, 0x40), (1, 3, 0x80)]
        for cx, cy, b in dots:
            if bits & b:
                px, py = X + 2 + cx * 4, Y + 2 + cy * 4.5
                d.rectangle([px, py, px + 2, py + 2], fill=fg)
        return
    SHAPES = {'█': (0, 0, 1, 1), '▬': (0, .38, 1, .62), '▰': (.1, .3, .9, .7), '▕': (.8, 0, 1, 1)}
    if ch in SHAPES:
        a, b, c, e = SHAPES[ch]
        d.rectangle([X + a * CW, Y + b * CH, X + c * CW - 1, Y + e * CH - 1], fill=fg); return
    if ch == '▱':
        d.rectangle([X + 1, Y + .3 * CH, X + CW - 2, Y + .7 * CH], outline=fg); return
    if ch == '░':
        for i in range(0, CW, 3):
            for j in range(0, CH, 3):
                d.point((X + i + (j // 3) % 2, Y + j), fill=fg)
        return
    if ch in BOX:
        s = BOX[ch]; cx, cy = X + CW // 2, Y + CH // 2
        w = 3 if any(c.isupper() for c in s) else 1
        for c in s.lower():
            if c == 'l': d.rectangle([X, cy, cx, cy + w - 1], fill=fg)
            if c == 'r': d.rectangle([cx, cy, X + CW - 1, cy + w - 1], fill=fg)
            if c == 'u': d.rectangle([cx, Y, cx + w - 1, cy], fill=fg)
            if c == 'd': d.rectangle([cx, cy, cx + w - 1, Y + CH - 1], fill=fg)
        return
    d.text((X, Y + 1), ch, font=BOLD if bold else FONT, fill=fg)

def render(path, pad=14):
    rows = parse(path)
    W = max(len(r) for r in rows)
    img = Image.new('RGB', (W * CW + pad * 2, len(rows) * CH + pad * 2), BG)
    sub = Image.new('RGB', (W * CW, len(rows) * CH), BG)
    d = ImageDraw.Draw(sub)
    for y, r in enumerate(rows):
        for x, c in enumerate(r):
            cell(d, x, y, *c)
    img.paste(sub, (pad, pad))
    return img

if __name__ == '__main__':
    for p in sys.argv[1:]:
        render(p).save(p.rsplit('.', 1)[0] + '.png', optimize=True)
