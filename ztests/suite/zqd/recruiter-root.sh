#!/bin/bash
#
# This script creates:
# (1) a zqd recruiter process
# (2) multiple zqd worker processes that register with the recruiter
# (3) a zqd root process that recruits from the workers and performs a distributed query
#
function awaitfile {
  file=$1
  i=0
  until [ -f $file ]; do
    let i+=1
    if [ $i -gt 5 ]; then
      echo "timed out waiting for file \"$file\" to appear"
      echo "zqd log:"
      cat zqd.log
      exit 1
    fi
    sleep 1
  done
}

zqdroot=$1
if [ -z "$zqdroot" ]; then
  zqdroot=zqdroot
fi
mkdir -p $zqdroot

mkdir -p s3/bucket
portdir=$(mktemp -d)
# (1) start a zqd recruiter process
zqd listen -personality=recruiter -l=localhost:0 -portfile="$portdir/zqd-rec" &> zqd-rec.log &
zqdrpid=$!
awaitfile $portdir/zqd-rec

# (2) start two zqd workers that register with the recruiter
ZQD_RECRUITER_ADDR=localhost:$(cat $portdir/zqd-rec) ZQD_NODE_NAME=test1 \
  zqd listen -personality=worker -l=localhost:0 -portfile="$portdir/zqd-w1" &> zqd-w1.log &
zqdw1pid=$!
awaitfile $portdir/zqd-w1

ZQD_RECRUITER_ADDR=localhost:$(cat $portdir/zqd-rec) ZQD_NODE_NAME=test1 \
  zqd listen -personality=worker -l=localhost:0 -portfile="$portdir/zqd-w2" &> zqd-w2.log &
zqdw1pid=$!
awaitfile $portdir/zqd-w2

# (3) start a zqd "root" process (default personality for now) that may recruit workers
ZQD_RECRUITER_ADDR=localhost:$(cat $portdir/zqd-rec) \
zqd listen -l=localhost:0 -portfile="$portdir/zqd-root" -data="$zqdroot" &> zqd-root.log &
zqdrootpid=$!
awaitfile $portdir/zqd-root
export ZQD_HOST=localhost:$(cat $portdir/zqd-root)

trap "rm -rf $portdir; kill -9 $zqdrpid $zqdw1pid $zqdw2pid $zqdrootpid" EXIT
