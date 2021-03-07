function wait_for_port {
    for i in {1..5}; do
    if nc -z localhost $1; then
        break
    fi
    sleep 1
    done
}

kubectl port-forward svc/zsrv-recruiter 8020:9867 &
kubectl port-forward svc/zsrv-root 9867:9867 &
kubectl port-forward svc/zsrv-zqd-temporal 9868:9867 &

wait_for_port 8020
wait_for_port 9867
wait_for_port 9868
