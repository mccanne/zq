#!/bin/bash

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

# first start a zqd recruiter process
zqd listen -personality=recruiter -l=localhost:0 -portfile="$portdir/zqd" -loglevel=warn &> zqd.log &
zqdpid=$!
awaitfile $portdir/zqd

# then start one zqd worker that will register with the recruiter
ZQD_RECRUITER_ADDR="127.0.0.1:$(cat $portdir/zqd)" ZQD_NODE_NAME=test1 ZQD_POD_IP=test1 ZQD_PORT=5000 \
  zqd listen  -personality=worker -l=localhost:0 -portfile="$portdir/zqd-w1" \
  -data="$zqdroot" -suricataupdater=true  -loglevel=warn &> zqd-w1.log &
zqdw1pid=$!
awaitfile $portdir/zqd-w1

trap "rm -rf $portdir; kill -9 $zqdpid $zqdw1pid" EXIT

export ZQD_HOST=localhost:$(cat $portdir/zqd)
