#!/usr/bin/env bash
# Regenerates the fixtures in this directory from upstream eccodes data and
# samples. Every file here is committed; this script only documents (and can
# reproduce) where each came from.
#
#   ECCODES=reference/eccodes-install/bin  # grib_set, grib_copy, ...
#   DATA=<dir with eccodes test data, see reference/eccodes/data/grib_data_files.txt>
#   ./testdata/upstream/generate.sh
set -euo pipefail
cd "$(dirname "$0")"
ROOT=../..
ECCODES=${ECCODES:-$ROOT/reference/eccodes-install/bin}
DATA=${DATA:?set DATA to a directory holding the eccodes test data files}
SAMPLES=$ROOT/reference/eccodes/samples
set_=$ECCODES/grib_set
copy=$ECCODES/grib_copy
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

# --- verbatim upstream data files ---------------------------------------
cp "$DATA"/test_uuid.grib2 .                          # NV=6 pv array
cp "$DATA"/multi_created.grib2 .                       # 1 message, 21 fields (S4 repeats)
cp "$DATA"/gfs.c255.grib2 .                            # 5.3 order 1, 3 extra octets, bitmap
cp "$DATA"/grid_ieee.grib grid_ieee.grib2              # PDT 4.40, IEEE
cp "$DATA"/reduced_latlon_surface_constant.grib2 .     # reduced lat/lon (Ni missing + PL)
cp "$DATA"/reduced_gaussian_sub_area.grib2 .           # reduced gaussian sub-area
cp "$DATA"/tigge_cf_ecmwf.grib2 .                      # bpv 1 and 24, bitmap
cp "$DATA"/tigge/tigge_egrr_sfc_2t.grib tigge_egrr_sfc_2t.grib2   # basic angle 360/864
cp "$DATA"/tigge/tigge_kwbc_pl_t.grib tigge_kwbc_pl_t.grib2       # bpv 0, D=1
cp "$DATA"/tigge/tigge_lfpw_pl_q.grib tigge_lfpw_pl_q.grib2       # bpv 0, D=-2
$copy -w count=1/2/3 "$DATA"/multi.grib2 multi_first3.grib2        # 5.40000 JPEG, S4 repeats

# --- derived with grib_set ----------------------------------------------
G=$SAMPLES/GRIB2.tmpl
S=$ROOT/testdata/sample.grib2
R=$ROOT/testdata/regular_latlon_surface.grib2

$set_ -s forecastTime=-2 $S fctime_neg2.grib2
$set_ -s productDefinitionTemplateNumber=8 $G $TMP/p8.grib2
$set_ -s forecastTime=-185 $TMP/p8.grib2 fctime_neg185_pdt8.grib2

$set_ -s typeOfFirstFixedSurface=1,scaledValueOfFirstFixedSurface=11 $G $TMP/sf.grib2
$set_ -s scaleFactorOfFirstFixedSurface=missing $TMP/sf.grib2 level_sf_missing.grib2

$set_ -s bitMapIndicator=254 $G bitmap_indicator_254.grib2
$set_ -s numberOfValues=2 $G numvalues_mismatch.grib2

$set_ -s tablesVersion=13,setCalendarId=1,typeOfCalendar=0 $G section1_calendar.grib2

$set_ -s decimalScaleFactor=3 $G constant_d3.grib2

$set_ -s productDefinitionTemplateNumber=8 $SAMPLES/reduced_gg_ml_grib2.tmpl pv_pdt8.grib2
for p in 5 40 113; do
  $set_ -s tablesVersion=36,productDefinitionTemplateNumber=$p $G raw_pdt$p.grib2
done

# --- projected grids (filters from tests/grib_grid_lambert_conformal.sh,
#     grib_grid_space_view.sh); constant fields so they stay tiny ---------
cat > $TMP/lam6.filt <<'F'
 set gridType="lambert";
 set numberOfDataPoints=294000;
 set shapeOfTheEarth=6;
 set Nx=588;
 set Ny=500;
 set latitudeOfFirstGridPoint=40442000;
 set longitudeOfFirstGridPoint=353559000;
 set LaD=60000000;
 set LoV=2200000;
 set Dx=2499000;
 set Dy=2499000;
 set Latin1=46401000;
 set Latin2=46401000;
 set numberOfValues=294000;
 write;
F
cat > $TMP/lam2.filt <<'F'
 set gridType="lambert";
 set numberOfDataPoints=294000;
 set shapeOfTheEarth=2;
 set Nx=588;
 set Ny=500;
 set latitudeOfFirstGridPoint=40442000;
 set longitudeOfFirstGridPoint=353559000;
 set LaD=60000000;
 set LoV=2200000;
 set Dx=2499000;
 set Dy=2499000;
 set Latin1=46401000;
 set Latin2=46401000;
 set numberOfValues=294000;
 write;
F
cat > $TMP/sv.filt <<'F'
 set gridType="space_view";
 set Nx=1900;
 set Ny=900;
 set dx=3622;
 set dy=3610;
 set Xp=764000;
 set Yp=1774000;
 set Nr=6610700;
 set numberOfDataPoints=1710000;
 set numberOfValues=1710000;
 write;
F
$ECCODES/grib_filter -o lambert_shape6.grib2 $TMP/lam6.filt $G
$ECCODES/grib_filter -o lambert_shape2.grib2 $TMP/lam2.filt $G
$ECCODES/grib_filter -o space_view_1900x900.grib2 $TMP/sv.filt $G
$set_ -s shapeOfTheEarth=5 $ROOT/testdata/mercator.grib2 mercator_shape5.grib2

# --- scanning-mode variants (tests/grib_change_scanning.sh) ---------------
# scanningMode=N only rewrites the flag, so eccodes reads the same data in a
# different order; swapScanningX/Y reorder the data and flip the flag.
for m in 16 32 64 128 192 48 96 224; do
  $set_ -s scanningMode=$m $R ll_scan$m.grib2
done
$set_ -s swapScanningX=1 $R ll_swapx.grib2
$set_ -s swapScanningY=1 $R ll_swapy.grib2
$set_ -s jScansPositively=1 $SAMPLES/regular_gg_sfc_grib2.tmpl $TMP/gg.grib2
$set_ -s swapScanningY=1 $ROOT/testdata/regular_gaussian_surface.grib2 gg_swapy.grib2
$set_ -s swapScanningX=1 $ROOT/testdata/regular_gaussian_surface.grib2 gg_swapx.grib2
$set_ -s swapScanningY=1 $ROOT/testdata/rdps_surface.grib2 $TMP/rdps_swapy.grib2 || true
$set_ -s angleOfRotationInDegrees=30 $SAMPLES/rotated_ll_sfc_grib2.tmpl rotated_angle30.grib2
$set_ -s scanningMode=64 $SAMPLES/rotated_ll_sfc_grib2.tmpl rotated_scan64.grib2
$set_ -s iScansNegatively=1 $ROOT/testdata/mercator.grib2 mercator_ineg.grib2
$set_ -s jScansPositively=1 $ROOT/testdata/mercator.grib2 mercator_jpos.grib2
# testdata/polar_stereographic.grib2 has Dx=Dy=0, so use the eccodes sample.
PS=$SAMPLES/polar_stereographic_pl_grib2.tmpl
cp $PS ps_sample.grib2
$set_ -s iScansNegatively=1 $PS ps_ineg.grib2
$set_ -s jScansPositively=1 $PS ps_jpos.grib2

# Small consistent Lambert / polar stereographic / space view grids.
cat > $TMP/lamsmall.filt <<'F'
 set gridType="lambert";
 set shapeOfTheEarth=6;
 set Nx=59; set Ny=50;
 set numberOfDataPoints=2950; set numberOfValues=2950;
 set latitudeOfFirstGridPoint=40442000; set longitudeOfFirstGridPoint=353559000;
 set LaD=60000000; set LoV=2200000;
 set Dx=24990000; set Dy=24990000;
 set Latin1=46401000; set Latin2=46401000;
 write;
F
$ECCODES/grib_filter -o lambert_small.grib2 $TMP/lamsmall.filt $G
$set_ -s iScansNegatively=1 lambert_small.grib2 lambert_small_ineg.grib2
$set_ -s jScansPositively=0 lambert_small.grib2 lambert_small_jneg.grib2
$set_ -s Latin2=30000000 lambert_small.grib2 lambert_small_secant.grib2
cat > $TMP/lamsouth.filt <<'F'
 set gridType="lambert";
 set shapeOfTheEarth=6;
 set Nx=20; set Ny=25;
 set numberOfDataPoints=500; set numberOfValues=500;
 set latitudeOfFirstGridPoint=-54387000; set longitudeOfFirstGridPoint=265669000;
 set LaD=-35000000; set LoV=295000000;
 set Dx=200000000; set Dy=200000000;
 set projectionCentreFlag=128;
 set Latin1=-35000000; set Latin2=-35000000;
 set latitudeOfSouthernPole=-90000000;
 set scanningMode=64;
 write;
F
$ECCODES/grib_filter -o lambert_south.grib2 $TMP/lamsouth.filt $G
$set_ -s LaD=-60000000,projectionCentreFlag=128,latitudeOfFirstGridPoint=-30000000 \
  $SAMPLES/polar_stereographic_pl_grib2.tmpl ps_south.grib2
cat > $TMP/svsmall.filt <<'F'
 set gridType="space_view";
 set Nx=190; set Ny=90;
 set dx=362; set dy=361;
 set Xp=76400; set Yp=177400;
 set Nr=6610700;
 set numberOfDataPoints=17100; set numberOfValues=17100;
 write;
F
$ECCODES/grib_filter -o space_view_small.grib2 $TMP/svsmall.filt $G
$set_ -s iScansNegatively=1 space_view_small.grib2 space_view_small_ineg.grib2
$set_ -s jScansPositively=1 space_view_small.grib2 space_view_small_jpos.grib2

# --- PNG at every byte depth (tests/grib_png.sh) ---------------------------
RGG=$ROOT/testdata/regular_gaussian_model_level.grib2
$set_ -r -s packingType=grid_png $RGG png_rgg.grib2
# (eccodes' PNG encoder fails on the 16x31 lat/lon field, so use this one.)
for b in 8 12 16 20 24 28 32; do
  $set_ -r -s setBitsPerValue=$b png_rgg.grib2 png_bpv$b.grib2
done
# PNG / complex with a bitmap, all missing, 6x2 bitmap with pad bits.
GCSD=$ROOT/testdata/grid_complex_spatial_differencing.grib2
$set_ -r -s packingType=grid_png $GCSD png_bitmap.grib2
$set_ -r -s packingType=grid_complex $GCSD complex_bitmap.grib2
$set_ -r -s packingType=grid_complex $R complex_rll.grib2
$set_ -r -s packingType=grid_complex_spatial_differencing,orderOfSpatialDifferencing=1 $R csd_order1.grib2
$set_ -r -s packingType=grid_complex_spatial_differencing,orderOfSpatialDifferencing=2 $R csd_order2.grib2
$set_ -s missingValueManagementUsed=2 $ROOT/testdata/gfs.complex.mvmu.grib2 $TMP/mvm2.grib2 && \
  $copy -w count=1 $TMP/mvm2.grib2 gfs_mvm2.grib2 || true
for b in 1 7 17 25 31 33 48 58; do
  $set_ -s setBitsPerValue=$b $S simple_bpv$b.grib2
done
$set_ -s decimalScaleFactor=-1 $R simple_dneg.grib2
$set_ -r -s packingType=grid_ieee,precision=2 $R ieee64.grib2
$set_ -r -s packingType=grid_ieee $GCSD ieee_bitmap.grib2

# --- statistically processed templates with several time ranges ----------
# (tests/grib_stattype.sh, grib_ecc-813.sh, grib_step.sh)
cat > $TMP/tr3.filt <<'F'
 set productDefinitionTemplateNumber = 8;
 set numberOfTimeRanges = 3;
 set typeOfStatisticalProcessing = {0,6,2};
 set indicatorOfUnitForTimeRange = {1,1,1};
 set lengthOfTimeRange = {720,24,1};
 set numberOfMissingInStatisticalProcess = 31;
 write;
F
$ECCODES/grib_filter -o pdt8_3ranges.grib2 $TMP/tr3.filt $G
cat > $TMP/tr11.filt <<'F'
 set productDefinitionTemplateNumber = 11;
 set numberOfTimeRanges = 2;
 set typeOfStatisticalProcessing = {0,6};
 set lengthOfTimeRange = {720,24};
 set typeOfTimeIncrement = {1,2};
 set timeIncrement = {24,1};
 set indicatorOfUnitForTimeIncrement = {1,1};
 set numberOfMissingInStatisticalProcess = 31;
 set perturbationNumber = 7;
 write;
F
$ECCODES/grib_filter -o pdt11_2ranges.grib2 $TMP/tr11.filt $G
$set_ -s productDefinitionTemplateNumber=12,numberOfTimeRanges=2 $G pdt12_2ranges.grib2
# Second fixed surface set (a layer): typeOfSecondFixedSurface=106 etc.
$set_ -s typeOfFirstFixedSurface=106,scaleFactorOfFirstFixedSurface=2,scaledValueOfFirstFixedSurface=13,typeOfSecondFixedSurface=106,scaleFactorOfSecondFixedSurface=2,scaledValueOfSecondFixedSurface=54 $G layer_106.grib2

# --- run-length encoded by eccodes (port of tests/grib_run_length_packing.cc)
cat > $TMP/rl.filt <<'F'
 set numberOfDataPoints = 4;
 set Ni = 2; set Nj = 2;
 set latitudeOfFirstGridPointInDegrees = 35;
 set longitudeOfFirstGridPointInDegrees = 135;
 set latitudeOfLastGridPointInDegrees = 35.025;
 set longitudeOfLastGridPointInDegrees = 135.016666;
 set iDirectionIncrementInDegrees = 0.0125;
 set jDirectionIncrementInDegrees = 0.008333;
 set jScansPositively = 1;
 set numberOfValues = 1;
 set packingType = "grid_run_length";
 set numberOfValues = 4;
 set bitsPerValue = 8;
 set maxLevelValue = 3;
 set numberOfLevelValues = 5;
 set decimalScaleFactor = 1;
 set levelValues = {101, 102, 103, 104, 105};
 set values = {9999, 10.1, 10.2, 10.3};
 write;
F
$ECCODES/grib_filter -o rl_enc.grib2 $TMP/rl.filt $G || echo "rl_enc failed" >&2

# --- longitudes past 360 and missing increments (tests/grib_ecc-1671.sh, grib_ecc-984.sh)
cat > $TMP/e1671.filt <<'F'
 set numberOfDataPoints = 5400000;
 set numberOfValues = 5400000;
 set Ni = 3600; set Nj = 1500;
 set latitudeOfFirstGridPoint = 89950000;
 set longitudeOfFirstGridPoint = 180050000;
 set latitudeOfLastGridPoint = -59950000;
 set longitudeOfLastGridPoint = 539950000;
 set iDirectionIncrement = 100000;
 set jDirectionIncrement = 100000;
 write;
F
$ECCODES/grib_filter -o ecc1671_lon540.grib2 $TMP/e1671.filt $G
$set_ -s ijDirectionIncrementGiven=0 $S ecc984_noincrements.grib2
$set_ -s ijDirectionIncrementGiven=0 $GCSD ecc984_greenwich_noinc.grib2

# --- Section 1 calendar template 2, raw PDTs with NV>0 and wave spectra --
$set_ -s tablesVersion=13,setCalendarId=1,calendarIdentificationTemplateNumber=2 $G section1_calendar2.grib2
$set_ -s tablesVersion=36,productDefinitionTemplateNumber=40 $SAMPLES/reduced_gg_ml_grib2.tmpl raw_pdt40_nv.grib2
$set_ -s tablesVersion=36,productDefinitionTemplateNumber=99,numberOfWaveDirections=5 $G raw_pdt99_wave.grib2 || echo "pdt99 failed" >&2

# --- constant fields from tigge and the eccodes samples ------------------
cp "$DATA"/tigge/tigge_kwbc_sfc_sm.grib tigge_kwbc_sfc_sm.grib2
cp $SAMPLES/ccsds_grib2.tmpl ccsds_sample.grib2
cp $SAMPLES/reduced_gg_sfc_jpeg_grib2.tmpl jpeg_sample_reduced_gg.grib2
$set_ -s productDefinitionTemplateNumber=11 $G $TMP/p11.grib2
$set_ -s forecastTime=-2 $TMP/p11.grib2 fctime_neg2_pdt11.grib2

# --- eccodes samples the tests use directly --------------------------------
mkdir -p gaussian_pl
for f in $SAMPLES/reduced_gg_pl_*_grib2.tmpl; do
  n=$(basename $f _grib2.tmpl); cp $f gaussian_pl/$n.grib2
done
cp $ROOT/reference/eccodes/ifs_samples/grib1_mlgrib2_ccsds/gg_ml.tmpl ifs_gg_ml_ccsds.grib2
cp $ROOT/reference/eccodes/ifs_samples/grib1_mlgrib2_ccsds/gg_sfc_grib2.tmpl ifs_gg_sfc_ccsds.grib2
