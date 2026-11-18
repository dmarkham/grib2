#!/usr/bin/env python3
"""Generate testdata/reference/projected_grids_proj.json: where PROJ puts
points on the grid of every Mercator, polar stereographic and Lambert
conformal fixture.

    python3 scripts/gen_projection_reference.py FIXTURE.grib2 [...] > testdata/reference/projected_grids_proj.json

For each fixture it reads the grid definition of the first message with
eccodes' grib_get (reference/eccodes-install/bin, or GRIB_GET=...), builds
the matching PROJ projection (pyproj) and records, for a fixed set of
points inside and outside the grid, the fractional canonical column and row
PROJ gives: column 0 is the western column, row 0 the northern row, as in
Field.Values. TestProjectedWeights_PROJ compares Sampler.Weights with them,
so the forward projections are checked against an independent
implementation without PROJ being needed to run the tests.

The fifth number of a point is PROJ's meridian convergence there in degrees
(the bearing of grid north, clockwise from true north), which
TestWindRotation_PROJ compares with WindRotation.GridNorth.

The first grid point is the first point in data order (WMO), so the i axis
runs west when scanning mode bit 1 is set and the j axis runs north when
bit 2 is set. Fixtures on other grids, and ones whose grid definition does
not describe the points they hold, are left out.
"""
import json
import math
import os
import random
import subprocess
import sys

from pyproj import Proj, __proj_version__

GRIB_GET = os.environ.get("GRIB_GET", "reference/eccodes-install/bin/grib_get")
COMMON = "numberOfDataPoints,latitudeOfFirstGridPoint,longitudeOfFirstGridPoint,LaD,scanningMode"
EARTH = "radius:d,earthMajorAxisInMetres:d,earthMinorAxisInMetres:d"
BY_TYPE = {
    "lambert": "Nx,Ny,Dx,Dy,LoV,Latin1,Latin2",
    "polar_stereographic": "Nx,Ny,Dx,Dy,orientationOfTheGrid,projectionCentreFlag",
    "mercator": "Ni,Nj,Di,Dj,orientationOfTheGrid",
}
INSIDE, AROUND = 200, 60


def get(path, keys):
    out = subprocess.run([GRIB_GET, "-f", "-F", "%.6f", "-w", "count=1", "-p", keys, path],
                         capture_output=True, text=True).stdout.split()
    if len(out) != len(keys.split(",")):
        return {}  # a fixture eccodes cannot read
    return dict(zip(keys.split(","), out))


def grid(path):
    kind = get(path, "gridType").get("gridType")
    if kind not in BY_TYPE:
        return None
    g = get(path, COMMON + "," + BY_TYPE[kind])
    if not g:
        return None
    num = {k: int(v) for k, v in g.items()}
    earth = get(path, EARTH)
    if not earth:
        return None
    if earth["radius:d"] != "not_found":
        shape = "+R=%s" % earth["radius:d"]
    else:
        shape = "+a=%s +b=%s" % (earth["earthMajorAxisInMetres:d"], earth["earthMinorAxisInMetres:d"])
    deg = lambda k: "%.6f" % (num[k] * 1e-6)
    if kind == "lambert":
        proj = "+proj=lcc +lat_1=%s +lat_2=%s +lat_0=%s +lon_0=%s" % (deg("Latin1"), deg("Latin2"), deg("LaD"), deg("LoV"))
        nx, ny, dx, dy = num["Nx"], num["Ny"], num["Dx"], num["Dy"]
    elif kind == "polar_stereographic":
        south = num["projectionCentreFlag"] & 0x80 != 0
        lat_ts = abs(num["LaD"] * 1e-6)  # the grid lengths hold at LaD, on the projection's own side
        proj = "+proj=stere +lat_0=%d +lat_ts=%.6f +lon_0=%s" % (-90 if south else 90, -lat_ts if south else lat_ts,
                                                                deg("orientationOfTheGrid"))
        nx, ny, dx, dy = num["Nx"], num["Ny"], num["Dx"], num["Dy"]
    else:
        proj = "+proj=merc +lat_ts=%s +lon_0=%s" % (deg("LaD"), deg("orientationOfTheGrid"))
        nx, ny, dx, dy = num["Ni"], num["Nj"], num["Di"], num["Dj"]
    if nx * ny != num["numberOfDataPoints"] or dx <= 0 or dy <= 0:
        return None  # not a usable grid (testdata/lambert.grib2, testdata/polar_stereographic.grib2)
    num.update(nx=nx, ny=ny, dx=dx * 1e-3, dy=dy * 1e-3)
    return num, proj + " " + shape + " +units=m +no_defs"


def reference(path):
    described = grid(path)
    if described is None:
        return None
    g, proj = described
    p = Proj(proj)
    nx, ny, dx, dy = g["nx"], g["ny"], g["dx"], g["dy"]
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
        conv = p.get_factors(lon, lat).meridian_convergence
        if all(map(math.isfinite, (lat, lon, x, y, conv))):
            points.append([lat, lon, (x - x_west) / dx, (y_north - y) / dy, conv])
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
