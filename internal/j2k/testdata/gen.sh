#!/bin/bash
# name|src|w,h,bits,s|opj args
set -u
OUT=out; mkdir -p $OUT; : > $OUT/manifest.txt
while IFS='|' read -r name src fmt args; do
  [ -z "$name" ] && continue
  case "$name" in \#*) continue;; esac
  IFS=, read -r w h b s <<<"$fmt"
  ncomp=1
  if ! opj_compress -i $src.raw -o $OUT/$name.j2k -F $w,$h,$ncomp,$b,$s $args >$OUT/$name.log 2>&1; then
     echo "COMPRESS FAIL $name"; cat $OUT/$name.log | tail -3; continue; fi
  if opj_decompress -i $OUT/$name.j2k -o $OUT/$name.dec.raw >>$OUT/$name.log 2>&1; then
     if [ $b -gt 8 ]; then dd if=$OUT/$name.dec.raw of=$OUT/$name.be.raw conv=swab status=none; else cp $OUT/$name.dec.raw $OUT/$name.be.raw; fi; if cmp -s $OUT/$name.be.raw $src.raw; then truth=lossless; else truth=lossy; fi
  else truth=opjfail; fi
  echo "$name $src $w $h $b $s $truth $(stat -c %s $OUT/$name.j2k) : $args" | tee -a $OUT/manifest.txt
done <<'LIST'
base|img12u|45,37,12,u|-n 4
base8|img8u|45,37,8,u|-n 4
base16|img16u|45,37,16,u|-n 4
signed16|img16s|45,37,16,s|-n 4
prec2|img1u|45,37,2,u|-n 3
n1|img12u|45,37,12,u|-n 1
n2|img12u|45,37,12,u|-n 2
n6|img12u|45,37,12,u|-n 6
prog_RLCP|img12u|45,37,12,u|-n 4 -p RLCP
prog_RPCL|img12u|45,37,12,u|-n 4 -p RPCL
prog_PCRL|img12u|45,37,12,u|-n 4 -p PCRL
prog_CPRL|img12u|45,37,12,u|-n 4 -p CPRL
sop|img12u|45,37,12,u|-n 4 -SOP
eph|img12u|45,37,12,u|-n 4 -EPH
sopeph|img12u|45,37,12,u|-n 4 -SOP -EPH
M1|img12u|45,37,12,u|-n 4 -M 1
M2|img12u|45,37,12,u|-n 4 -M 2
M4|img12u|45,37,12,u|-n 4 -M 4
M8|img12u|45,37,12,u|-n 4 -M 8
M16|img12u|45,37,12,u|-n 4 -M 16
M32|img12u|45,37,12,u|-n 4 -M 32
M5|img12u|45,37,12,u|-n 4 -M 5
M17|img12u|45,37,12,u|-n 4 -M 17
M20|img12u|45,37,12,u|-n 4 -M 20
M38|img12u|45,37,12,u|-n 4 -M 38
M63|img12u|45,37,12,u|-n 4 -M 63
cb4|img12u|45,37,12,u|-n 4 -b 4,4
cb16|img12u|45,37,12,u|-n 4 -b 16,16
cb8x32|img12u|45,37,12,u|-n 4 -b 8,32
cb1024x4|img12u|45,37,12,u|-n 4 -b 1024,4
prc_LRCP|img12u|45,37,12,u|-n 4 -c [16,16],[8,8] -b 4,4
prc_RLCP|img12u|45,37,12,u|-n 4 -c [16,16],[8,8] -b 4,4 -p RLCP
prc_RPCL|img12u|45,37,12,u|-n 4 -c [16,16],[8,8] -b 4,4 -p RPCL
prc_PCRL|img12u|45,37,12,u|-n 4 -c [16,16],[8,8] -b 4,4 -p PCRL
prc_CPRL|img12u|45,37,12,u|-n 4 -c [16,16],[8,8] -b 4,4 -p CPRL
prc_layers|img12u|45,37,12,u|-n 4 -c [16,16],[8,8] -b 4,4 -p PCRL -r 8,3,1
layers_r|img12u|45,37,12,u|-n 4 -r 10,4,1
layers_q|img12u|45,37,12,u|-n 4 -q 30,40,0
layers_M1|img12u|45,37,12,u|-n 4 -r 10,4,1 -M 1
layers_M4|img12u|45,37,12,u|-n 4 -r 10,4,1 -M 4
layers_M63|img12u|45,37,12,u|-n 4 -r 10,4,1 -M 63
tiles|img12u|45,37,12,u|-n 3 -t 16,16
tiles_odd|img12u|45,37,12,u|-n 3 -t 17,13
tiles_n4|img12u|45,37,12,u|-n 4 -t 32,32
offset|img12u|45,37,12,u|-n 4 -d 3,5
offset_t|img12u|45,37,12,u|-n 3 -d 3,5 -t 16,16 -T 1,2
offset_odd|img12u|45,37,12,u|-n 4 -d 7,1
tpR|img12u|45,37,12,u|-n 4 -TP R
tpL|img12u|45,37,12,u|-n 4 -r 10,4,1 -TP L
tiles_tpR|img12u|45,37,12,u|-n 3 -t 16,16 -TP R
tlm|img12u|45,37,12,u|-n 3 -t 16,16 -TLM
plt|img12u|45,37,12,u|-n 4 -PLT
poc1|img12u|45,37,12,u|-n 4 -r 10,4,1 -POC T1=0,0,2,4,1,RLCP/T1=0,0,3,4,1,LRCP
poc2|img12u|45,37,12,u|-n 4 -r 10,4,1 -POC T1=0,0,1,2,1,LRCP/T1=0,0,3,4,1,RLCP
poc_prc|img12u|45,37,12,u|-n 4 -r 10,4,1 -c [16,16],[8,8] -b 4,4 -POC T1=0,0,1,4,1,PCRL/T1=0,0,3,4,1,RPCL
roi|img12u|45,37,12,u|-n 4 -ROI c=0,U=4
gb1|img12u|45,37,12,u|-n 4 -GuardBits 1
gb4|img12u|45,37,12,u|-n 4 -GuardBits 4
lossy|img12u|45,37,12,u|-n 4 -r 8
lossy_layers|img12u|45,37,12,u|-n 4 -r 20,8
w1|w1|1,37,12,u|-n 1
h1|h1|37,1,12,u|-n 1
one|one|1,1,12,u|-n 1
zero|zero|64,64,8,u|-n 4
irr|img12u|45,37,12,u|-n 4 -I
sub|img12u|45,37,12,u|-n 4 -s 2,2
LIST
