#!/usr/bin/env python3
"""Generate testdata/reference/lambert_proj.json: where PROJ puts points on
the grid of every Lambert conformal fixture.

    python3 scripts/gen_lambert_reference.py FIXTURE.grib2 [...] > testdata/reference/lambert_proj.json

For each fixture it reads the grid definition of the first message with
eccodes' grib_get (reference/eccodes-install/bin, or GRIB_GET=...), builds
the matching PROJ projection (pyproj) and records, for a fixed set of
points inside and outside the grid, the fractional canonical column and row
PROJ gives: column 0 is the western column, row 0 the northern row, as in
Field.Values. TestLambertWeights_PROJ compares Sampler.Weights with them, so
the forward projection is checked against an independent implementation
without PROJ being needed to run the tests.

The first grid point is the first point in data order (WMO), so the i axis
runs west when scanning mode bit 1 is set and the j axis runs north when
bit 2 is set.
"""
import json
import math
import os
import random
import subprocess
import sys

from pyproj import Proj, __proj_version__

GRIB_GET = os.environ.get("GRIB_GET", "reference/eccodes-install/bin/grib_get")
KEYS = ("numberOfDataPoints,Nx,Ny,latitudeOfFirstGridPoint,longitudeOfFirstGridPoint,LaD,LoV,"
        "Latin1,Latin2,Dx,Dy,scanningMode,radius,earthMajorAxisInMetres,earthMinorAxisInMetres")
INSIDE, AROUND = 200, 60


def grid(path):
    out = subprocess.run([GRIB_GET, "-f", "-w", "count=1", "-p", KEYS, path],
                         check=True, capture_output=True, text=True).stdout.split()
    g = dict(zip(KEYS.split(","), out))
    num = {k: int(v) for k, v in g.items() if v != "not_found"}
    if "radius" in num:
        shape = "+R=%d" % num["radius"]
    else:
        shape = "+a=%s +b=%s" % (g["earthMajorAxisInMetres"], g["earthMinorAxisInMetres"])
    deg = lambda k: "%.6f" % (num[k] * 1e-6)
    proj = "+proj=lcc +lat_1=%s +lat_2=%s +lat_0=%s +lon_0=%s %s +units=m +no_defs" % (
        deg("Latin1"), deg("Latin2"), deg("LaD"), deg("LoV"), shape)
    return num, proj


def reference(path):
    g, proj = grid(path)
    if g["Nx"] * g["Ny"] != g["numberOfDataPoints"]:
        return None  # not a usable grid (testdata/lambert.grib2)
    p = Proj(proj)
    nx, ny, dx, dy = g["Nx"], g["Ny"], g["Dx"] * 1e-3, g["Dy"] * 1e-3
    i_neg, j_pos = g["scanningMode"] & 0x80 != 0, g["scanningMode"] & 0x40 != 0
    x1, y1 = p(g["longitudeOfFirstGridPoint"] * 1e-6, g["latitudeOfFirstGridPoint"] * 1e-6)
    # Plane position of canonical column 0 (west) and row 0 (north).
    x_west = x1 - (nx - 1) * dx if i_neg else x1
    y_north = y1 + (ny - 1) * dy if j_pos else y1

    rnd = random.Random(path)
    cells = [(rnd.uniform(0, nx - 1), rnd.uniform(0, ny - 1)) for _ in range(INSIDE)]
    cells += [(rnd.uniform(-0.5 * nx, 1.5 * nx), rnd.uniform(-0.5 * ny, 1.5 * ny)) for _ in range(AROUND)]
    points = []
    for col, row in cells:
        lon, lat = p(x_west + col * dx, y_north - row * dy, inverse=True)
        lat, lon = round(lat, 5), round(lon, 5)  # the query, exactly as stored
        x, y = p(lon, lat)
        if all(map(math.isfinite, (lat, lon, x, y))):
            points.append([lat, lon, (x - x_west) / dx, (y_north - y) / dy])
    return {"proj": proj, "points": points}


def main():
    fixtures = []
    for f in sorted(sys.argv[1:]):
        r = reference(f)
        if r is None:
            continue
        pts = ",\n".join(json.dumps(pt) for pt in r["points"])
        fixtures.append('%s: {"proj": %s, "points": [\n%s\n]}' % (json.dumps(f), json.dumps(r["proj"]), pts))
    print('{"proj_version": %s, "fixtures": {\n%s\n}}' % (json.dumps(__proj_version__), ",\n".join(fixtures)))


if __name__ == "__main__":
    main()
