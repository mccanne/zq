#!/bin/bash
# This is designed to be run from the zq root directory
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

make install
helm uninstall zsrv
make docker-push-ecr

sleep 5 # wait for ECR

make helm-install-with-aurora-temporal

sleep 30 # wait for services

$DIR/zqd-port.sh

aws s3 rm --recursive s3://brim-scratch/mark/sp-m1 || :
zapi new -k archivestore -d s3://brim-scratch/mark/sp-m1 -thresh 5MB sp-m1
zapi -s sp-m1 post s3://brim-scratch/mark/conn.log.gz

zapi -s sp-m1 get -t "count()"
zapi -s sp-m1 get -t "39161"

make test-cluster

export AWS_DEFAULT_REGION=us-east-2
export AWS_ACCESS_KEY_ID=$(aws configure get aws_access_key_id)
export AWS_SECRET_ACCESS_KEY=$(aws configure get aws_secret_access_key)

make test-temporal

# To add more workers:
# kubectl scale --replicas=3 deployment/worker-zqd
