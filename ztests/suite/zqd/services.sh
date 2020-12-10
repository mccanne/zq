#!/bin/bash

function awaitfile {
  file=$1
  i=0
  until [ -f $file ]; do
    let i+=1
    if [ $i -gt 5 ]; then
      echo "timed out waiting for file \"$file\" to appear"
      echo "minio log:"
      cat minio.log
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

minio server --writeportfile=$portdir/minio --quiet --address localhost:0 ./s3 > minio.log 2>&1 &
miniopid=$!
awaitfile $portdir/minio

# AWS env variables must be set before zqd starts so zqd has access to them.
export AWS_REGION=does-not-matter
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_S3_ENDPOINT=http://localhost:$(cat $portdir/minio)

if [[ "$2" == workers ]]; then
  # start two zqd workers and a zqd root process
  zqd listen -l=localhost:0 -portfile=$portdir/zqd-w1 -data=$zqdroot -loglevel=warn -suricataupdater=true &> zqd-w1.log &
  zqdw1pid=$!
  zqd listen -l=localhost:0 -portfile=$portdir/zqd-w2 -data=$zqdroot -loglevel=warn -suricataupdater=true &> zqd-w2.log &
  zqdw2pid=$!

  awaitfile $portdir/zqd-w1
  portw1=$(cat $portdir/zqd-w1)

  awaitfile $portdir/zqd-w2
  portw2=$(cat $portdir/zqd-w2)

  test_workers=127.0.0.1:$portw1,127.0.0.1:$portw2
  zqd listen -l=localhost:0 -portfile=$portdir/zqd -data=$zqdroot \
    -loglevel=warn -suricataupdater=true -workers=$test_workers &> zqd-root.log &
  zqdpid=$!
  awaitfile $portdir/zqd
else
  zqd listen -l=localhost:0 -portfile=$portdir/zqd -data=$zqdroot -loglevel=warn -suricataupdater=true &> zqd.log &
  zqdpid=$!
  awaitfile $portdir/zqd
fi
trap "rm -rf $portdir; kill -9 $miniopid $zqdpid $zqdw1pid $zqdw2pid" EXIT

export ZQD_HOST=localhost:$(cat $portdir/zqd)
