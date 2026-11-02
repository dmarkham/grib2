#!/bin/bash
# Regenerates the testdata/jpeg_* fixtures used by packing_jpeg2000_test.go.
# Needs the eccodes build in reference/eccodes-install (with both OpenJPEG
# and Jasper) and opj_decompress. Run from the repository root.
set -euo pipefail
E=reference/eccodes-install/bin
export ECCODES_DEFINITION_PATH=$PWD/reference/eccodes-install/share/eccodes/definitions
export ECCODES_SAMPLES_PATH=$PWD/reference/eccodes-install/share/eccodes/samples
T=testdata
W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT

vals() { # file -> one value per line (nan for missing), %.17g, decoded by OpenJPEG
  ECCODES_GRIB_JPEG=openjpeg $E/grib_get_data -m nan -F %.17g "$1" | awk 'NR>1 {print $3}' > "${1%.grib2}.values"
}

# H3: constant fields (bpv 0) with D != 0 decode to R, not R*10^-D.
ECCODES_GRIB_JPEG=openjpeg $E/grib_set -s packingType=grid_jpeg,decimalScaleFactor=3 \
  $ECCODES_SAMPLES_PATH/GRIB2.tmpl $T/jpeg_const_d3.grib2
ECCODES_GRIB_JPEG=openjpeg $E/grib_set -s packingType=grid_jpeg,decimalScaleFactor=-2 \
  $ECCODES_SAMPLES_PATH/GRIB2.tmpl $T/jpeg_const_dm2.grib2

# M26: external streams above 14 bits (and 1 bit), from both libraries.
for lib in openjpeg jasper; do
  for b in 1 24 30 31; do
    [ $lib = jasper ] && [ $b -ge 30 ] && continue
    ECCODES_GRIB_JPEG=$lib $E/grib_set -r -s packingType=grid_jpeg \
      $T/regular_latlon_surface.grib2 $W/j.grib2
    ECCODES_GRIB_JPEG=$lib $E/grib_set -s setBitsPerValue=$b $W/j.grib2 $T/jpeg_bpv${b}_$lib.grib2
  done
done

# M26: tile-part QCC and 6 resolutions on a 16x31 grid (Jasper and OpenJPEG).
for lib in openjpeg jasper; do
  ECCODES_GRIB_JPEG=$lib $E/grib_set -r -s packingType=grid_jpeg \
    $T/regular_latlon_surface.grib2 $T/jpeg_rls_$lib.grib2
done

# H20: lossy (typeOfCompressionUsed=1). OpenJPEG writes a reversible 5/3
# stream truncated in rate; Jasper writes an irreversible 9/7 stream.
for lib in openjpeg jasper; do
  ECCODES_GRIB_JPEG=$lib $E/grib_set -r -s typeOfCompressionUsed=1,targetCompressionRatio=10 \
    $T/jpeg.grib2 $T/jpeg_lossy_$lib.grib2
done

# M9: JPEG with a bitmap (varying and constant), from the 16x31 grid with
# every 7th point (and the first and last) missing.
$E/grib_get_data -F %.17g $T/regular_latlon_surface.grib2 | awk 'NR>1 {print $3}' > $W/v.txt
awk 'BEGIN{ORS=""; print "set bitmapPresent=1; set missingValue=9999; set values={"}
     {v=$1; if ((NR-1)%7==3 || NR==1 || NR==496) v=9999; printf "%s%s", (NR>1?",":""), v}
     END{print "}; write;\n"}' $W/v.txt > $W/bm.filter
ECCODES_GRIB_JPEG=openjpeg $E/grib_filter -o $W/bm.grib2 $W/bm.filter $T/regular_latlon_surface.grib2
ECCODES_GRIB_JPEG=openjpeg $E/grib_set -r -s packingType=grid_jpeg $W/bm.grib2 $T/jpeg_bitmap.grib2
awk 'BEGIN{ORS=""; print "set bitmapPresent=1; set missingValue=9999; set values={"}
     {v=273.5; if ((NR-1)%7==3 || NR==1 || NR==496) v=9999; printf "%s%s", (NR>1?",":""), v}
     END{print "}; write;\n"}' $W/v.txt > $W/bmc.filter
ECCODES_GRIB_JPEG=openjpeg $E/grib_filter -o $W/bmc.grib2 $W/bmc.filter $T/regular_latlon_surface.grib2
ECCODES_GRIB_JPEG=openjpeg $E/grib_set -r -s packingType=grid_jpeg $W/bmc.grib2 $T/jpeg_bitmap_const.grib2

# L9: ECC-802, a 2x2 grid with a negative minimum.
printf "set Ni=2; set Nj=2; set bitsPerValue=12; set packingType='grid_jpeg';\nset values={-0.01, 11.99, 56.11, 98.99}; write;\n" > $W/ecc802.filter
ECCODES_GRIB_JPEG=openjpeg $E/grib_filter -o $T/jpeg_ecc802.grib2 $W/ecc802.filter $ECCODES_SAMPLES_PATH/GRIB2.tmpl 2>/dev/null

# M9: a 6x2 grid whose 12-bit bitmap ends in pad bits, missing at 7 and 8.
printf "set Ni=6; set Nj=2; set bitmapPresent=1; set missingValue=9999; set bitsPerValue=12;\nset packingType='grid_jpeg';\nset values={1.5,2,3,4,5,6,7,9999,9999,10,11,12.25}; write;\n" > $W/bm6x2.filter
ECCODES_GRIB_JPEG=openjpeg $E/grib_filter -o $T/jpeg_bm6x2.grib2 $W/bm6x2.filter $ECCODES_SAMPLES_PATH/GRIB2.tmpl 2>/dev/null

# M9: all points missing (empty Section 7) on the 16x31 grid.
awk 'BEGIN{ORS=""; print "set bitmapPresent=1; set missingValue=9999; set values={"}
     {printf "%s%s", (NR>1?",":""), 9999} END{print "}; write;\n"}' $W/v.txt > $W/am.filter
ECCODES_GRIB_JPEG=openjpeg $E/grib_filter -o $W/am.grib2 $W/am.filter $T/regular_latlon_surface.grib2
ECCODES_GRIB_JPEG=openjpeg $E/grib_set -r -s packingType=grid_jpeg $W/am.grib2 $T/jpeg_allmissing.grib2

# No .values for jpeg_bpv30/31_openjpeg: OpenJPEG wraps the QCD exponents
# (32, 33 do not fit in 5 bits) and its own decode is off by up to 33 K.
for f in $T/jpeg_ecc802.grib2 $T/jpeg_bm6x2.grib2 $T/jpeg_allmissing.grib2 $T/jpeg_const_d3.grib2 $T/jpeg_const_dm2.grib2 \
         $T/jpeg_bpv1_*.grib2 $T/jpeg_bpv24_*.grib2 $T/jpeg_rls_*.grib2 \
         $T/jpeg_lossy_openjpeg.grib2 $T/jpeg_bitmap.grib2 $T/jpeg_bitmap_const.grib2; do
  vals $f
done
# The lossy OpenJPEG field is 65160 values; keep only the SHA-256 of the
# exact codestream samples (int32 little-endian) from opj_decompress.
rm -f $T/jpeg_lossy_openjpeg.values
python3 - "$T/jpeg_lossy_openjpeg.grib2" "$W" <<'PY' > $T/jpeg_lossy_openjpeg.sha256
import glob, hashlib, struct, subprocess, sys
d = open(sys.argv[1], 'rb').read()
open(sys.argv[2] + '/l.j2k', 'wb').write(d[d.find(b'\xff\x4f\xff\x51'):len(d) - 4])
subprocess.run(['opj_decompress', '-i', sys.argv[2] + '/l.j2k', '-o', sys.argv[2] + '/l.pgx'],
               check=True, capture_output=True)
p = open(glob.glob(sys.argv[2] + '/l*.pgx')[0], 'rb').read()
body = p[p.index(b'\n') + 1:]
n = len(body) // 2
v = [int.from_bytes(body[2 * i:2 * i + 2], 'big') for i in range(n)]
print(hashlib.sha256(b''.join(struct.pack('<i', x) for x in v)).hexdigest())
PY

# M26: reference values for the existing reduced Gaussian JPEG fixture,
# whose codestream carries a tile-part QCC.
ECCODES_GRIB_JPEG=openjpeg $E/grib_get_data -m nan -F %.17g $T/reduced_gaussian_surface_jpeg.grib2 |
  awk 'NR>1 {print $3}' > $T/jpeg_reduced_gaussian_surface.values
