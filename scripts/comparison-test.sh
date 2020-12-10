#!/bin/bash
# shellcheck disable=SC2016    # The backticks in quotes are for markdown, not expansion

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR" || exit 1

DATA="../zq-sample-data"
ln -sfn zeek-default "$DATA/zeek"
ln -sfn zeek-ndjson "$DATA/ndjson"

if [[ $(type -P "gzcat") ]]; then
  ZCAT="gzcat"
elif [[ $(type -P "zcat") ]]; then
  ZCAT="zcat"
else
  echo "gzcat/zcat not found in PATH"
  exit 1
fi

for CMD in zq jq zeek-cut; do
  if ! [[ $(type -P "$CMD") ]]; then
    echo "$CMD not found in PATH"
    exit 1
  fi
done

declare -a MARKDOWNS=(
    '01_all_unmodified.md'
    '02_cut_ts.md'
    '03_count_all.md'
    '04_count_by_id_orig_h.md'
    '05_only_id_resp_h.md'
)

declare -a DESCRIPTIONS=(
    'Output all events unmodified'
    'Extract the field `ts`'
    'Count all events'
    'Count all events, grouped by the field `id.orig_h`'
    'Output all events with the field `id.resp_h` set to `52.85.83.116`'
)

declare -a ZQL_QUERIES=(
    '*'
    'cut ts'
    'count()'
    'count() by id.orig_h'
    'id.resp_h=52.85.83.116'
)

declare -a JQ_FILTERS=(
    '.'
    '. | { ts }'
    '. | length'
    'group_by(."id.orig_h")[] | length as $l | .[0] | .count = $l | {count,"id.orig_h"}'
    '. | select(.["id.resp_h"]=="52.85.83.116")'
)
declare -a JQFLAGS=(
    '-c'
    '-c'
    '-c -s'
    '-c -s'
    '-c'
)
declare -a ZCUT_FIELDS=(
    ''
    'ts'
    'NONE'
    'NONE'
    'NONE'
)

for (( n=0; n<"${#ZQL_QUERIES[@]}"; n++ ))
do
    DESC=${DESCRIPTIONS[$n]}
    MD=${MARKDOWNS[$n]}
    zql=${ZQL_QUERIES[$n]}
    echo -e "### $DESC\n" | tee "$MD"
    echo "|**<br>Tool**|**<br>Arguments**|**Input<br>Format**|**Output<br>Format**|**<br>Real**|**<br>User**|**<br>Sys**|" | tee -a "$MD"
    echo "|:----------:|:---------------:|:-----------------:|:------------------:|-----------:|-----------:|----------:|" | tee -a "$MD"
    for INPUT in zeek zng zng-uncompressed tzng ndjson ; do
      for OUTPUT in zeek zng zng-uncompressed tzng ndjson ; do
        echo -n "|\`zq\`|\`$zql\`|$INPUT|$OUTPUT|" | tee -a "$MD"
        if [[ $INPUT == "zng-uncompressed" ]]; then
          INPUT_FMT="zng"
        else
          INPUT_FMT="$INPUT"
        fi
        if [[ $OUTPUT == "zng-uncompressed" ]]; then
          ALL_TIMES=$(time -p (zq -i "$INPUT_FMT" -f zng -znglz4blocksize 0 "$zql" $DATA/$INPUT/* > /dev/null) 2>&1)
        else
          ALL_TIMES=$(time -p (zq -i "$INPUT_FMT" -f "$OUTPUT" "$zql" $DATA/$INPUT/* > /dev/null) 2>&1)
        fi
        echo "$ALL_TIMES" | tr '\n' ' ' | awk '{ print $2 "|" $4 "|" $6 "|" }' | tee -a "$MD"
      done
    done

    ZCUT=${ZCUT_FIELDS[$n]}
    if [[ $ZCUT != "NONE" ]]; then
      echo "|\`zeek-cut\`|\`$ZCUT\`|zeek|zeek-cut|" | sed 's/\`\`//' | tr -d '\n' | tee -a "$MD"
      ALL_TIMES=$(time -p ($ZCAT "$DATA"/zeek/* | zeek-cut "$ZCUT" > /dev/null) 2>&1)
      echo "$ALL_TIMES" | tr '\n' ' ' | awk '{ print $2 "|" $4 "|" $6 "|" }' | tee -a "$MD"
    fi

    JQ=${JQ_FILTERS[$n]}
    JQFLAG=${JQFLAGS[$n]}
    echo -n "|\`jq\`|\`$JQFLAG ""'""${JQ//|/\\|}""'""\`|ndjson|ndjson|" | tee -a "$MD"
    # shellcheck disable=SC2086      # For expanding JQFLAG
    ALL_TIMES=$(time -p ($ZCAT "$DATA"/zeek-ndjson/* | jq $JQFLAG "$JQ" > /dev/null) 2>&1)
    echo "$ALL_TIMES" | tr '\n' ' ' | awk '{ print $2 "|" $4 "|" $6 "|" }' | tee -a "$MD"

    echo | tee -a "$MD"
done
