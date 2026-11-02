#!/bin/bash
# Generates the testdata/ccsds_*.grib2 fixtures and their expected values
# (grib_get_data -F%.17g, missing as nan) with the eccodes build in reference/.
# Values files that equal ccsds_base.grib2.values are not kept (see
# packing_ccsds_test.go).
set -euo pipefail
R=$(cd "$(dirname "$0")/.." && pwd)
E=$R/reference/eccodes-install/bin
SAMP=$R/reference/eccodes-install/share/eccodes/samples/GRIB2.tmpl
OUT=${OUT:-$R/testdata}
W=$(mktemp -d "${TMPDIR:-$OUT}/ccsdsfx.XXXXXX")
vals() { # n formula-in-awk
  awk -v n=$1 "BEGIN{for(i=0;i<n;i++){ $2 ; printf \"%s%.9g\", (i?\",\":\"\"), v } }"
}
# 16x16 smooth field with noise and a flat patch.
V=$(vals 256 'x=i%16; y=int(i/16); v=273.15+25*sin(x/3.0)*cos(y/4.0)+((i*7919)%13)*0.11; if(i>=96&&i<160) v=280.25')
cat > $W/base.filt <<F
set Ni=16; set Nj=16;
set latitudeOfLastGridPoint=45000000; set longitudeOfLastGridPoint=15000000;
set iDirectionIncrement=1000000; set jDirectionIncrement=1000000;
set packingType="grid_ccsds";
set bitsPerValue=14;
set ccsdsBlockSize=16; set ccsdsRsi=2; set ccsdsFlags=14;
set values={$V};
write;
F
$E/grib_filter -o $OUT/ccsds_base.grib2 $W/base.filt $SAMP
for N in 4 12 13 20 29 36 44; do $E/grib_set -r -s ccsdsFlags=$N $OUT/ccsds_base.grib2 $OUT/ccsds_flags$N.grib2; done
for B in 8 64; do $E/grib_set -r -s ccsdsBlockSize=$B $OUT/ccsds_base.grib2 $OUT/ccsds_bs$B.grib2; done
for X in 1 7 4096; do $E/grib_set -r -s ccsdsRsi=$X $OUT/ccsds_base.grib2 $OUT/ccsds_rsi$X.grib2; done
for B in 17 20 24 25 31 32; do $E/grib_set -r -s setBitsPerValue=$B $OUT/ccsds_base.grib2 $OUT/ccsds_bpv$B.grib2; done
# IEEE -> CCSDS (ECC-1202), gives bpv 32
$E/grib_set -r -s packingType=grid_ieee $OUT/ccsds_base.grib2 $W/ieee.grib2
$E/grib_set -r -s packingType=grid_ccsds $W/ieee.grib2 $OUT/ccsds_from_ieee.grib2
# optimizeScaleFactor (ECC-2088): D!=0, very negative E
$E/grib_set -r -s optimizeScaleFactor=1 $OUT/ccsds_base.grib2 $OUT/ccsds_optscale.grib2
# Constant field with D=2 (H3): bpv 0 must decode to R, not R*10^-D
cat > $W/constd2.filt <<F
set Ni=16; set Nj=16;
set latitudeOfLastGridPoint=45000000; set longitudeOfLastGridPoint=15000000;
set iDirectionIncrement=1000000; set jDirectionIncrement=1000000;
set packingType="grid_ccsds";
set decimalScaleFactor=2;
set values={$(vals 256 'v=5.5')};
write;
F
$E/grib_filter -o $OUT/ccsds_const_d2.grib2 $W/constd2.filt $SAMP
# ECC-802: tiny grid with a negative minimum
cat > $W/ecc802.filt <<F
set Ni=2; set Nj=2;
set bitsPerValue=12;
set packingType="grid_ccsds";
set values={-0.01, 11.99, 56.11, 98.99};
write;
F
$E/grib_filter -o $OUT/ccsds_ecc802.grib2 $W/ecc802.filt $SAMP
# Tiny values (ECC-1388 style)
cat > $W/tiny.filt <<F
set Ni=16; set Nj=16;
set latitudeOfLastGridPoint=45000000; set longitudeOfLastGridPoint=15000000;
set iDirectionIncrement=1000000; set jDirectionIncrement=1000000;
set packingType="grid_ccsds";
set bitsPerValue=24;
set values={$(vals 256 'v=1e-25*(1+i/1000.0)')};
write;
F
$E/grib_filter -o $OUT/ccsds_tiny.grib2 $W/tiny.filt $SAMP
# Bitmap fixtures (M9): 9999 is the missing value
VB=$(vals 256 'x=i%16; y=int(i/16); v=273.15+25*sin(x/3.0)*cos(y/4.0)+((i*7919)%13)*0.11; if((i*37)%11==0||(i>=200&&i<215)) v=9999')
cat > $W/bitmap.filt <<F
set Ni=16; set Nj=16;
set latitudeOfLastGridPoint=45000000; set longitudeOfLastGridPoint=15000000;
set iDirectionIncrement=1000000; set jDirectionIncrement=1000000;
set packingType="grid_ccsds";
set bitsPerValue=16;
set bitmapPresent=1; set missingValue=9999;
set values={$VB};
write;
F
$E/grib_filter -o $OUT/ccsds_bitmap.grib2 $W/bitmap.filt $SAMP
VC=$(vals 256 'v=287.5; if((i*37)%11==0) v=9999')
sed "s/set values=.*/set values={$VC};/" $W/bitmap.filt > $W/bitmapc.filt
$E/grib_filter -o $OUT/ccsds_bitmap_const.grib2 $W/bitmapc.filt $SAMP
VM=$(vals 256 'v=9999')
sed "s/set values=.*/set values={$VM};/" $W/bitmap.filt > $W/allmiss.filt
$E/grib_filter -o $OUT/ccsds_allmissing.grib2 $W/allmiss.filt $SAMP
# 6x2 grid: bitmap not byte aligned (12 points), missing at 7 and 8
cat > $W/bm6x2.filt <<F
set Ni=6; set Nj=2;
set packingType="grid_ccsds";
set bitsPerValue=12;
set bitmapPresent=1; set missingValue=9999;
set values={1,2,3,4,5,6,7,9999,9999,10,11,12};
write;
F
$E/grib_filter -o $OUT/ccsds_bm6x2.grib2 $W/bm6x2.filt $SAMP
# Expected values
for f in $OUT/ccsds_*.grib2; do
  [ "$(basename $f)" = ccsds.grib2 ] && continue
  if $E/grib_get_data -m nan -F%.17g $f > $W/vals 2>/dev/null; then
    awk 'NR>1{print $3}' $W/vals > $f.values
  else
    echo error > $f.values
  fi
done
rm -rf $W
# Drop expected-value files that duplicate another one; the tests map them.
for f in bpv17 bpv20 bpv24 bpv25 bpv31 bpv32 bs8 bs64 flags4 flags12 flags20 from_ieee rsi1 rsi7 rsi4096; do
  cmp -s $OUT/ccsds_$f.grib2.values $OUT/ccsds_base.grib2.values && rm $OUT/ccsds_$f.grib2.values
done
cmp -s $OUT/ccsds_flags29.grib2.values $OUT/ccsds_flags13.grib2.values && rm $OUT/ccsds_flags29.grib2.values
rm -f $OUT/ccsds_const_d2.grib2.values $OUT/ccsds_allmissing.grib2.values $OUT/ccsds_flags36.grib2.values
