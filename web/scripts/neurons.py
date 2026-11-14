# The neuron drawings of the drawn designs (styles/sketch.css), in the manner
# of a careful ink illustration: every branch is a filled shape that tapers
# toward its tip, spines have small heads, the soma is solid ink with a light
# nucleus, and small letters label the parts as on an anatomical plate. Fixed
# seeds: the same drawings every time. Run: python3 web/scripts/neurons.py
import math, os, random

ART = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'src', 'styles', 'art')

def f(v):
    return ('%.1f' % v).rstrip('0').rstrip('.')

def curve(pts, start=True):
    """Catmull-Rom through points as cubic Béziers (M at the start if asked)."""
    d = f'M{f(pts[0][0])} {f(pts[0][1])}' if start else f'L{f(pts[0][0])} {f(pts[0][1])}'
    for i in range(len(pts) - 1):
        p0 = pts[max(0, i - 1)]; p1 = pts[i]; p2 = pts[i + 1]; p3 = pts[min(len(pts) - 1, i + 2)]
        c1 = (p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6)
        c2 = (p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6)
        d += f'C{f(c1[0])} {f(c1[1])} {f(c2[0])} {f(c2[1])} {f(p2[0])} {f(p2[1])}'
    return d

def ribbon(pts, widths):
    """A tapered stroke: the outline of a line whose width changes along it."""
    left, right = [], []
    for i, (x, y) in enumerate(pts):
        a = pts[max(0, i - 1)]; b = pts[min(len(pts) - 1, i + 1)]
        dx, dy = b[0] - a[0], b[1] - a[1]
        n = math.hypot(dx, dy) or 1
        nx, ny = -dy / n, dx / n
        w = widths[i] / 2
        left.append((x + nx * w, y + ny * w))
        right.append((x - nx * w, y - ny * w))
    poly = left + list(reversed(right))
    area = sum(poly[i][0] * poly[(i + 1) % len(poly)][1] - poly[(i + 1) % len(poly)][0] * poly[i][1] for i in range(len(poly)))
    if area < 0:
        left, right = right, left
    tip = pts[-1]
    return curve(left) + f'L{f(tip[0])} {f(tip[1])}' + curve(list(reversed(right)), start=False) + 'Z'

class Drawing:
    def __init__(self, seed):
        self.r = random.Random(seed)
        self.ink = []      # filled shapes
        self.fine = []     # thin strokes (spine necks)
        self.dots = []     # spine heads, nucleoli
        self.somata = []   # cell bodies, with the nucleus left light
        self.labels = []   # (letter, x, y, leader from, leader to)
        self.pts = []

    def branch(self, x, y, angle, length, w0, w1, depth, maxdepth, spread=(16, 34), spines=0.5, bend=0.14, kids=(2, 2, 3), shrink=(0.6, 0.8), wshrink=0.72, up=None):
        r = self.r
        steps = max(4, int(length / 5))
        pts = [(x, y)]
        a = angle
        for i in range(steps):
            a += r.uniform(-bend, bend) + (angle - a) * 0.3
            if up is not None:
                a += (up - a) * 0.08  # leaning toward a direction (Purkinje fans grow up)
            seg = length / steps
            x += math.cos(a) * seg
            y += math.sin(a) * seg
            pts.append((x, y))
        widths = [w0 + (w1 - w0) * (i / steps) ** 0.8 for i in range(len(pts))]
        self.ink.append(ribbon(pts, widths))
        self.pts += pts
        # Spines along thin branches: a neck and a small head.
        if spines and w0 < 1.6:
            for i in range(1, len(pts) - 1):
                if r.random() < spines:
                    px, py = pts[i]
                    sa = a + r.choice((-1, 1)) * r.uniform(1.0, 1.8)
                    sl = r.uniform(1.3, 2.6)
                    ex, ey = px + math.cos(sa) * sl, py + math.sin(sa) * sl
                    self.fine.append(f'M{f(px)} {f(py)}L{f(ex)} {f(ey)}')
                    if r.random() < 0.7:
                        self.dots.append((ex, ey, r.uniform(0.45, 0.75)))
        if depth < maxdepth and w1 > 0.32:
            n = r.choice(kids)
            for k in range(n):
                sign = 1 if k % 2 == 0 else -1
                da = math.radians(r.uniform(*spread)) * sign * (1 + 0.35 * (k // 2))
                self.branch(x, y, a + da, length * r.uniform(*shrink), w1, max(0.3, w1 * wshrink * r.uniform(0.75, 0.9)),
                            depth + 1, maxdepth, spread, spines, bend, kids, shrink, wshrink, up)
        else:
            # A fine end that fades out.
            pass

    def soma(self, cx, cy, rx, ry, points=9, top=0.0, nucleus=0.42):
        r = self.r
        outline = []
        for i in range(points):
            t = i / points * 2 * math.pi
            k = 1 + r.uniform(-0.1, 0.1)
            y = cy + math.sin(t) * ry * k
            # Pyramidal somata point upward.
            if top and math.sin(t) < -0.6:
                y -= top * (-math.sin(t) - 0.6)
            outline.append((cx + math.cos(t) * rx * k, y))
        loop = outline + outline[:3]
        d = curve(loop[1:-1]) + 'Z'
        # The nucleus shows as a light ring, the nucleolus as a dot in it.
        nr = min(rx, ry) * nucleus
        nx, ny = cx + r.uniform(-0.6, 0.6), cy + r.uniform(-0.4, 0.6)
        d += f'M{f(nx - nr)} {f(ny)}a{f(nr)} {f(nr * 0.92)} 0 1 0 {f(2 * nr)} 0a{f(nr)} {f(nr * 0.92)} 0 1 0 {f(-2 * nr)} 0Z'
        self.somata.append(d)
        dr = nr * 0.32
        self.dots.append((nx + nr * 0.15, ny - nr * 0.1, dr))
        self.pts += outline

    def axon(self, x, y, angle, length, w0, w1, collaterals=3):
        """The axon: long, thin, nearly straight; fine collaterals leave it at
        a few places, and it ends in a small arbor of terminals."""
        r = self.r
        steps = max(6, int(length / 5))
        pts = [(x, y)]
        a = angle
        for i in range(steps):
            a += r.uniform(-0.05, 0.05) + (angle - a) * 0.2
            x += math.cos(a) * length / steps
            y += math.sin(a) * length / steps
            pts.append((x, y))
        widths = [w0 + (w1 - w0) * (i / steps) for i in range(len(pts))]
        self.ink.append(ribbon(pts, widths))
        self.pts += pts
        for c in range(collaterals):
            i = int(len(pts) * (0.3 + 0.5 * c / max(1, collaterals - 1)))
            px, py = pts[i]
            side = 1 if c % 2 == 0 else -1
            self.branch(px, py, a + side * r.uniform(0.9, 1.3), r.uniform(18, 30), widths[i] * 0.7, 0.3, 3, 4, (25, 40), 0, 0.18, (1, 2), (0.5, 0.65), 0.8)
        for k in range(3):
            self.branch(x, y, a + (k - 1) * r.uniform(0.4, 0.7), r.uniform(7, 12), w1, 0.28, 4, 5, (25, 40), 0, 0.2, (1, 2), (0.5, 0.7), 0.8)

    def svg(self, margin=6):
        xs = [p[0] for p in self.pts]; ys = [p[1] for p in self.pts]
        box = f'{f(min(xs) - margin)} {f(min(ys) - margin)} {f(max(xs) - min(xs) + 2 * margin)} {f(max(ys) - min(ys) + 2 * margin)}'
        out = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{box}">',
               f'<path fill="#000" d="{"".join(self.ink)}"/>']
        for soma in self.somata:
            out.append(f'<path fill="#000" fill-rule="evenodd" d="{soma}"/>')
        if self.fine:
            out.append(f'<path fill="none" stroke="#000" stroke-width="0.45" stroke-linecap="round" d="{"".join(self.fine)}"/>')
        if self.dots:
            out.append('<path fill="#000" d="' + ''.join(f'M{f(x - r)} {f(y)}a{f(r)} {f(r)} 0 1 0 {f(2 * r)} 0a{f(r)} {f(r)} 0 1 0 {f(-2 * r)} 0' for x, y, r in self.dots) + '"/>')
        for letter, x, y, (ax, ay), (bx, by) in self.labels:
            out.append(f'<path fill="none" stroke="#000" stroke-width="0.4" d="M{f(ax)} {f(ay)}L{f(bx)} {f(by)}"/>')
            out.append(f'<text x="{f(x)}" y="{f(y)}" font-family="Georgia, \'Times New Roman\', serif" font-style="italic" font-size="9">{letter}</text>')
        out.append('</svg>')
        return '\n'.join(out) + '\n'

# A pyramidal cell.
p = Drawing(21)
cx, cy = 110, 190
p.soma(cx, cy, 12, 9, points=11, top=12, nucleus=0.36)
p.branch(cx, cy - 12, -math.pi / 2, 74, 4.2, 2.2, 0, 4, (14, 28), 0.45, 0.12)
for ang, ln in ((160, 46), (205, 40), (20, 44), (-25, 38), (125, 36), (60, 40), (95, 30)):
    t = math.radians(ang)
    p.branch(cx + math.cos(t) * 8, cy + math.sin(t) * 6, t, ln, 2.6, 1.3, 1, 3, (18, 38), 0.5, 0.16)
p.axon(cx + 1, cy + 7, math.pi / 2 + 0.05, 118, 1.5, 0.55, 3)
p.labels += [('a', cx + 30, cy - 18, (cx + 28, cy - 20), (cx + 9, cy - 6)),
             ('b', cx + 22, cy + 70, (cx + 20, cy + 67), (cx + 4, cy + 60)),
             ('c', cx - 30, cy - 52, (cx - 24, cy - 54), (cx - 3, cy - 50))]
open(os.path.join(ART, 'neuron.svg'), 'w').write(p.svg())

# A Purkinje cell: a round soma, one thick dendrite that splits into a dense,
# flat fan of fine branches growing upward.
k = Drawing(5)
cx, cy = 120, 230
k.soma(cx, cy, 11, 12, points=11, nucleus=0.34)
for lean in (-0.35, 0.3):
    k.branch(cx + lean * 10, cy - 10, -math.pi / 2 + lean, 34, 4.4, 3.0, 0, 7, (16, 30), 0.38, 0.1, (2, 2, 3), (0.62, 0.8), 0.7, up=-math.pi / 2)
k.axon(cx, cy + 11, math.pi / 2 - 0.03, 80, 1.5, 0.6, 2)
k.labels += [('a', cx + 32, cy + 6, (cx + 30, cy + 3), (cx + 12, cy)),
             ('b', cx - 34, cy + 48, (cx - 28, cy + 46), (cx - 2, cy + 42))]
open(os.path.join(ART, 'purkinje.svg'), 'w').write(k.svg())

# A dendrite for under the wordmark: a gentle wave that thins out, short
# side branches and spines.
d = Drawing(3)
line = [(x, 9 + 1.5 * math.sin(x / 21) + d.r.uniform(-0.2, 0.2)) for x in range(0, 158, 4)]
d.ink.append(ribbon(line, [1.9 - 1.5 * (i / len(line)) ** 1.3 for i in range(len(line))]))
d.pts += line
for i in range(3, len(line) - 4, 5):
    x, y = line[i]
    a = (-math.pi / 2 if d.r.random() < 0.5 else math.pi / 2) + d.r.uniform(-0.8, 0.8)
    d.branch(x, y, a, d.r.uniform(4, 7), 1.0, 0.35, 3, 3, (30, 50), 0.6, 0.2)
open(os.path.join(ART, 'dendrite.svg'), 'w').write(d.svg(margin=2))
for n in ('neuron.svg', 'purkinje.svg', 'dendrite.svg'):
    print(n, len(open(os.path.join(ART, n)).read()))
