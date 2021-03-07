# Setup instructions for Temporal

Temporal is installed as a subchart of our zservice Helm "umbrella" chart.

Our Helm subchart is based on:
https://github.com/temporalio/helm-charts
With minimal changes.

Each user that will run Temporal must have an initialized database available. We use Aurora for the per-user Temporal databases. Setting up a Temporal database on the zq-test-aurora instance involves manual steps, because the Temporal database initialization and migration code is not designed to be run automated. (We confirmed this with the the Temporal dev team.)

Temporal has a concept of namespaces. A namespace must be registered before use. An external utility called tctl is used for that. There are instructions below for doing this after the Helm deploy.

## Helm chart for Temporal
`https://github.com/temporalio/helm-charts`
includes a helm chart for Temporal, which includes dependencies on:
Cassandra, ElasticSearch, Kafka (with Zookeeper), Promethueus, Grafana
We use the helm chart following a pattern similar to:
https://github.com/temporalio/helm-charts#install-with-your-own-postgresql
And avoid all the dependencies above.

`temporalio/helm-charts` is not designed to be used as a subchart within an umbrella Helm chart, and thus is not available in a helm repository. Brim uses it as a subchart, so we have followed a strategy similar to what is described here:
https://medium.com/@mattiaperi/create-a-public-helm-chart-repository-with-github-pages-49b180dbb417
In order to create a respository that we can use for our subchart dependency on Temporal.
We have created a git repo:
https://github.com/brimsec/helm-chart
That includes the repo `https://github.com/temporalio/helm-charts` as a git submodule.
The steps to do this, in case they need to be repeated, are:
```
git clone https://github.com/brimsec/helm-chart.git
cd helm-chart/helm-chart-sources
git submodule add https://github.com/temporalio/helm-charts.git temporal
cd temporal
git checkout v1.7.0
helm dependency update
cd ../..
helm package helm-chart-sources/*
git status # should have untracked file: temporal-0.2.2.tgz
helm repo index --url https://brimsec.github.io/helm-chart/ .
# repo index creates the index.yaml file that is needed to "publish" the chart
git add .
git commit -m "new index"
git push
```
Assuming that all the steps from Mattia Peri's article above have been taken, the Temporal helm chart will be available from the brim repo:
```
helm repo add brim https://brimsec.github.io/helm-chart/
helm search repo temporal
```
Will show the newly added Temporal helm chart.

## Database initialization for Temporal
We will follow a pattern similar to what we have done for setting up Aurora users for testing.

We will use a Docker container with the most recent build of `temporal-sql-tool`. Here's how to create that. Note that we are working with Temporal v1.7.0. The git clone and the docker build steps can be skipped
if a Docker image has already been pushed to $ZQD_ECR_HOST/temporal:1.7.0.

```
git clone https://github.com/temporalio/temporal.git
git checkout v1.7.0
docker build -t temporal .
aws ecr get-login-password --region us-east-2 | \
  docker login --username AWS --password-stdin $ZQD_ECR_HOST/temporal
docker tag temporal $ZQD_ECR_HOST/temporal:1.7.0
docker push $ZQD_ECR_HOST/temporal:1.7.0
kubectl run -i --tty --rm temporal-sql --image=$ZQD_ECR_HOST/temporal:1.7.0 -- sh
```
If you have envsubst installed, an alternate way of starting the pod is:
```
envsubst < k8s/temporal-sql.yaml | kubectl apply -f -
kubectl exec --stdin --tty temporal-sql -- sh
```

In the shell for the pod, use these commands, adapted for your username, to set up the Temporal databases. Note that every database name must be qualified by prefixing a username, because we need seperate Temporal databases for test isolation.

```
export SQL_PLUGIN=postgres
export SQL_PORT=5432
export SQL_HOST=<RDS host>
export SQL_USER=theusername
export SQL_PASSWORD=thepassword

temporal-sql-tool create-database -database theusername_temporal
SQL_DATABASE=theusername_temporal temporal-sql-tool setup-schema -v 0.0
SQL_DATABASE=theusername_temporal temporal-sql-tool update -schema-dir schema/postgresql/v96/temporal/versioned

temporal-sql-tool create-database -database theusername_temporal_visibility
SQL_DATABASE=theusername_temporal_visibility temporal-sql-tool setup-schema -v 0.0
SQL_DATABASE=theusername_temporal_visibility temporal-sql-tool update -schema-dir schema/postgresql/v96/visibility/versioned
```

## Using Makefile rule to deploy temporal
Temporal may be deployed as part of the zservice Helm chart. Use:
```
make helm-install-with-aurora-temporal
```
Prior to using this target, you must set additional environment variables that are used to configure Temporal DB access:
```
TEMPORAL_DATABASE=theusername_temporal
TEMPORAL_VISIBILITY_DATABASE=theusername_temporal_visibility
ZQD_AURORA_USER=theusername
ZQD_AURORA_PW=$(kubectl get secret aurora --template="{{ index .data \"postgresql-password\" }}" | base64 --decode)
ZQD_AURORA_HOST=$(aws rds describe-db-cluster-endpoints \
		--db-cluster-identifier zq-test-aurora \
		--output text --query "DBClusterEndpoints[?EndpointType=='WRITER'] | [0].Endpoint"):5432
```
Note that by convention, we qualify the database names with the username. This is to allow test isolation between deployments of Temporal.

## Use tctl to register a namespace for ztests
After Temporal has been deployed, and it finds the Postgres database and is status "Running", a namespace must be registered before the Ztests will run.

The Temporal image which contains the temporal-sql-tool also incudes tctl. It must be run from this image to have access to the temporal front-end in the Kubernetes cluster. Start a pod the same as for the sql tool:
```
kubectl run -i --tty -rm temporal-sql --image=$ZQD_ECR_HOST/temporal:1.7.0 -- sh
```
And in the interactive shell for the pod use the command:
```
tctl --address zsrv-temporal-frontend:7233 --ns zqd-ztest-persistent namespace register
```
It should respond with: 
```
Namespace zqd-ztest-persistent successfully registered.
```
This registers the namespace that is used for the temporal cluster Ztests.
