import math, struct, sys
def img(w, h, bits, signed, seed=12345):
    x = seed
    out = []
    lo = -(1 << (bits-1)) if signed else 0
    hi = (1 << (bits-1)) - 1 if signed else (1 << bits) - 1
    for j in range(h):
        for i in range(w):
            x = (x * 1103515245 + 12345) & 0x7fffffff
            noise = (x >> 16) % max(1, (hi - lo) // 16 + 1)
            v = (math.sin(i / 5.0) * math.cos(j / 7.0) * 0.4 + 0.5) * (hi - lo) * 0.85 + lo + noise
            v = int(v)
            v = min(hi, max(lo, v))
            out.append(v)
    return out
def write(name, w, h, bits, signed, vals):
    with open(name, 'wb') as f:
        for v in vals:
            if bits <= 8:
                f.write(struct.pack('b' if signed else 'B', v))
            else:
                f.write(struct.pack('>h' if signed else '>H', v))
specs = [
    ('img12u', 45, 37, 12, False),
    ('img8u', 45, 37, 8, False),
    ('img16u', 45, 37, 16, False),
    ('img16s', 45, 37, 16, True),
    ('img1u', 45, 37, 1, False),
    ('w1', 1, 37, 12, False),
    ('h1', 37, 1, 12, False),
    ('one', 1, 1, 12, False),
]
for name, w, h, bits, s in specs:
    write(name + '.raw', w, h, bits, s, img(w, h, bits, s))
write('zero.raw', 64, 64, 8, False, [0]*64*64)
