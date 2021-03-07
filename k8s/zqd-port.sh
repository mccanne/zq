#!/bin/bash
set -x #echo on
# start by killing existing port-forwards
kill $(ps aux | grep '[p]ort-forward' | awk '{print $2}')
sleep 1 # wait a second for the processes to get the signal
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
source $DIR/port-forward-temporal.sh
curl http://localhost:8020/recruiter/listfree
zapi ls 
