"""Rig the WellD "D" mascot: split each drawing into parts respawn can move.

Every movable part is a <g data-part="..."> with its pivot in style (transform-origin, in
user units). Eyes are holes cut by a mask, so they can look around and blink; ears are a
clipped copy of the head outline, so they can twitch. "RSP" in ids is replaced per
instance by respawn.js.
"""
import re
import sys

SRC, OUT = sys.argv[1], sys.argv[2]


def paths(f):
    s = open(f).read()
    out = []
    for p in re.findall(r'<path [^>]*/>', s):
        d = re.search(r' d="([^"]*)"', p).group(1)
        fill = re.search(r' fill="([^"]*)"', p).group(1)
        out.append((d, fill))
    return out


def subpaths(d):
    return ['M' + x for x in d.split('M') if x.strip()]


def poly(pts):
    return 'M' + ' L'.join(f'{x} {y}' for x, y in pts) + 'Z'


def g(part, pivot, inner, extra=''):
    return f'<g data-part="{part}" style="transform-origin:{pivot[0]}px {pivot[1]}px"{extra}>{inner}</g>'


def P(d, fill):
    return f'<path d="{d}" fill="{fill}"/>'


FIGS = {
    'hello': dict(src='b4604140-D-versione-1.svg', center=(188, 117), box=(411, 517), wave='arm-r',
                  pivots={'all': (200, 517), 'head': (190, 228), 'ear-l': (42, 44), 'ear-r': (330, 36),
                          'eye-l': (95, 143), 'eye-r': (285, 138), 'arm-r': (290, 300), 'arm-l': (88, 322),
                          'body': (197, 517)},
                  ears={'ear-l': [(66, 10), (66, -20), (-20, -20), (-20, 82), (17, 82)],
                        'ear-r': [(301, 2), (301, -20), (430, -20), (430, 73), (360, 73)]},
                  seams={'ear-l': ((17, 82), (66, 10)), 'ear-r': ((301, 2), (360, 73))},
                  holes={'eye-l': 1, 'eye-r': 2}, highlights={'eye-r': [0, 1], 'eye-l': [2, 3]},
                  order=[('head',), ('arm-r', [5]), ('arm-l', [6]), ('body', [7])]),
    'peace': dict(src='ef558eec-D-versione-2.svg', center=(140, 105), box=(395, 396), wave='arm-r',
                  pivots={'all': (200, 396), 'head': (150, 205), 'ear-l': (34, 88), 'ear-r': (250, 30),
                          'eye-l': (93, 155), 'eye-r': (234, 117), 'arm-l': (187, 243), 'arm-r': (226, 228),
                          'body': (186, 190)},
                  ears={'ear-l': [(47, 53), (47, -20), (-20, -20), (-20, 140), (20, 123)],
                        'ear-r': [(222, 8.8), (222, -20), (320, -20), (320, 53.8), (281, 53.8)]},
                  seams={'ear-l': ((20, 123), (47, 53)), 'ear-r': ((222, 8.8), (281, 53.8))},
                  holes={'eye-l': 1, 'eye-r': 2}, highlights={'eye-r': [0, 1], 'eye-l': [2, 3]},
                  order=[('head',), ('arm-l', [5]), ('arm-r', [6]), ('body', [7])]),
    'puppet': dict(src='4770b829-D-versione-3.svg', center=(217, 405), box=(431, 728), wave='',
                   pivots={'all': (180, 0), 'head': (215, 490), 'ear-l': (114, 338), 'ear-r': (322, 365),
                           'eye-l': (140, 415), 'eye-r': (274, 433), 'arm-l': (97, 535), 'arm-r': (283, 556),
                           'leg-l': (150, 616), 'leg-r': (214, 630), 'body': (195, 566)},
                   ears={'ear-l': [(137, 315), (137, 290), (60, 290), (60, 361), (92, 361)],
                         'ear-r': [(313, 334), (313, 300), (370, 300), (370, 396), (332, 396)]},
                   seams={'ear-l': ((92, 361), (137, 315)), 'ear-r': ((313, 334), (332, 396))},
                   holes={'eye-l': 2, 'eye-r': 1},
                   highlights={'eye-l': [1, 3, 5, 7, 9, 11, 13, 15, 17], 'eye-r': [2, 4, 6, 8, 10, 12, 14, 16, 18]},
                   order=[('head',), ('stripes', [19, 20]), ('arm-l', [21, 22, 23, 24]), ('leg-r', [26, 27, 28, 29]),
                          ('arm-r', [30, 31, 25, 33]), ('body', [32]), ('leg-l', [34, 35, 36, 37]),
                          ('strings', [38, 39, 40])]),
}

for name, F in FIGS.items():
    ps = paths(f'{SRC}/{F["src"]}')
    W, H = F['box']
    head_d = [d for d, _ in ps if d.count('M') == 3][0]
    outline, *holes = subpaths(head_d)
    holes = [None] + holes  # holes[1], holes[2] as in FIGS
    piv = F['pivots']
    defs = []
    defs.append(f'<path id="RSP-head" d="{outline}"/>')
    defs.append(f'<clipPath id="RSP-head-clip"><use href="#RSP-head"/></clipPath>')
    mask = [f'<rect x="-50" y="-50" width="{W + 100}" height="{H + 100}" fill="white"/>']
    for ear, pts in F['ears'].items():
        mask.append(P(poly(pts), 'black'))
        # the ear copy reaches a little into the head, so a turned ear leaves no gap
        cx, cy = F['center']
        def inward(pt, k=6):
            dx, dy = cx - pt[0], cy - pt[1]
            n = (dx * dx + dy * dy) ** .5
            return (round(pt[0] + dx / n * k, 1), round(pt[1] + dy / n * k, 1))
        clip = [inward(pts[0])] + pts[1:-1] + [inward(pts[-1])]
        defs.append(f'<clipPath id="RSP-{ear}"><path d="{poly(clip)}"/></clipPath>')
    for eye in ('eye-l', 'eye-r'):
        inner = P(holes[F['holes'][eye]], 'black') + ''.join(P(ps[i][0], 'white') for i in F['highlights'][eye])
        mask.append(g(eye, piv[eye], inner))
    defs.append(f'<mask id="RSP-face" maskUnits="userSpaceOnUse" x="-50" y="-50" width="{W + 100}" height="{H + 100}">{"".join(mask)}</mask>')

    seams = ''.join(f'<path d="M{a[0]} {a[1]} L{b[0]} {b[1]}" stroke="white" stroke-width="6" fill="none"/>'
                    for a, b in F['seams'].values())
    ears = ''.join(g(ear, piv[ear], f'<use href="#RSP-head" fill="white" clip-path="url(#RSP-{ear})"/>') for ear in F['ears'])
    head = g('head', piv['head'],
             f'<use href="#RSP-head" fill="white" mask="url(#RSP-face)"/>'
             f'<g clip-path="url(#RSP-head-clip)">{seams}</g>{ears}')
    body = []
    for item in F['order']:
        if item[0] == 'head':
            body.append(head)
            continue
        part, idx = item
        inner = ''.join(P(*ps[i]) for i in idx)
        body.append(g(part, piv.get(part, (W / 2, H / 2)), inner) if part not in ('stripes', 'strings') else f'<g data-part="{part}">{inner}</g>')
    svg = (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" class="rsp-figure" '
           f'data-figure="{name}" data-wave="{F["wave"]}" overflow="visible">'
           f'<defs>{"".join(defs)}</defs>{g("all", piv["all"], "".join(body))}</svg>\n')
    open(f'{OUT}/d-{name}.svg', 'w').write(svg)
    print(name, len(svg))
