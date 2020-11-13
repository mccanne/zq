# zqd recruit

zqd recruit manages a pool of zqd workers and provides a mechanism for zqd root processes to recruit zqd worker processes for query execution. It is designed to work well in a Kubernetes cluster, but it is not directly dependent on K8s APIs, and can function identically in other environments. This allows local development and ZTest scripts to be independent of K8s APIs.

The zqd recruit command is similar to zqd listen in that it starts an HTTP REST server. When zqd workers and root processes are started with the zqd listen command, they must be provided with the endpoint of the zqd recruit process. (Note that in a K8s cluster all the zqd processes will be run in containers within pods, but that is not mandatory for testing.)

## API

At a high level, zqd recruit provides the following API. In K8s, these will be published as a service, but again, that is not needed for tests.

/register is called by a worker process after it has started and is capable of processing /worker messages. In the /register message, the worker provides its endpoint to the recruiter process. It may also provide other details that are relevant to work scheduling, such as the node on which a K8s pod for the worker resides, or information about the state of the cached data files available to the worker. /register will be called again when a zqd process completes a /worker request and is available to be recruited by another zqd root process for query execution. In order to make the scheduling system more robust, /register will be called again after a zqd worker process has been idle (e.g. not processing /worker requests) for a given timeout period. This will allow a failed zqd recruiter process to gradually recover state in case of an unexpected restart.

/recruit is called by a zqd root process prior to starting query execution (i.e. /search). The number of worker processes desired is passed as a parameter. Other details relevant to scheduling, such as the desired cache state for the workers, will be included in future implementations. The response to the /recruit request will include the endpoints for the available workers, which may be less than the number of workers requested. In addition, we can implement desirable scheduling heuristics: for example, if a /recruit requests W workers, we can attempt to select the available workers evenly from the N nodes in a cluster (i.e. no more than W/N workers per node).

These two messages are the core of the API. The implementation behind them is not complicated: when the recruiter process receives a `/register` request, it adds that worker to a pool of available workers. Conversely, when the recruiter processes a `/recruit` request, it removes the recruited worker from the pool. Both `/register` and `/recruit` must be thread safe. At present, we do not have a requirement to maintain information about zqd workers that are 'busy' processing requests -- from the point of view of the zqd recruiter process, they are simply forgotten until they register again. This simple implementation allows the zqd worker instances to manage their own lifecycle, and terminate and restart when appropriate.

At least two additional messages will be needed to account for both expected and unexpected termination of zqd processes:

/unregister will be called by a zqd worker process that gracefully terminates while “believing” that it is registered with the zqd recruiter.

/noshow will be called by a zqd root process that is not able to establish contact with a zqd worker that has been identified in the response to a /recruit request. /noshow will have the side effect of /unregister for the given zqd worker.

We may find there are other failure cases that require additional messages. The general goal is that the registry of zqd workers is eventually consistent with the zqd worker states, and that inconsistencies will not lead to catastrophic outcomes, in any case.

## Need for a database

The initial implementation of zqd recruit will not require an external database. The use of the /register request by zqd workers will ensure that the available pool is eventually consistent with the state of the zqd workers. In a K8s cluster, high availability of the zqd recruit process will be based on its ability to rapidly restart and recover state from incoming /register requests.

Future implementations will use an external Redis database. This will have the advantage of allowing a zqd recruiter instance to more quickly recover all state after an unexpected restart. It will also allow us to run more than one instance of a zqd recruiter process per cluster. The CPU requirements and memory requirements for a zqd recruiter process are likely to be small, but we may find that having extra instances improves availability.

The main reason to introduce Redis will come when we introduce SSD caches for nodes running zqd workers. We can then use cache state (e.g. the availability of desired S3 objects) to preferentially schedule zqd workers on a given node. This heuristic for scheduling will require up-to-date information on the cache state of each node in the cluster, and a Redis database is a convenient way to share information on recent cache state.

## The "Redis only" option

If we rely on the presence of Redis, it would be possible to implement the algorithms suggested above without a zqd recruit process. This would shift more of the scheduling logic over to the zqd root process that is executing the /search command. Some logic for database consistency would need to be implemented as Redis scripts or modules. Overall, I think this would be a harder initial implementation than adding a zqd recruit command, but it is something to consider in the future.

## Future ideas for autoscaling

The zqd recruiter process can monitor the rate of /recruit request and the size of the available pool. If the pool is smaller or larger than desired for a given period of time, the zqd recruiter can trigger a scaling process for the zqd worker instances. For example, it can adjust the number of replicas in a K8s deployment. This type of autoscaling will be more targeted and precise for our needs than the behavior of the K8s Horizontal Pod Autoscaler (HPA).

## Future topic: "Cache striping"

When we implement SSD caches for K8s cluster nodes that host zqd workers, we will want to implement scheduling heuristics that make it more likely that S3 objects are cached in such a way that searches will be executed in parallel. As an example, suppose that we are performing a search that engages W worker processes. As mentioned above, we would like the workers to run on different nodes, to spread the CPU load. In addition, when workers have a "cache miss" and must perform an S3 GET, we would like the retrieved S3 objects to be "spread evenly" across the available nodes. It would be optimal for S3 objects corresponding to consecutive time spans of data to be cached on different nodes. Ideally, the cached S3 object for a long time span, say N consecutive S3 objects, would be cached on N separate nodes. This is analogous to "striping" data on RAID arrays. The scheduling heuristics for the zqd recruiter process will influence how the SSD cache can be best utilized.
