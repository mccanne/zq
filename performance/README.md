# Performance

The tables below provide a summary of simple operations and how `zq`
performs at them relative to `zeek-cut` and `jq`. All operations were performed
on a Google Cloud `n1-standard-8` VM (8 vCPUs, 30 GB memory) with the logs
stored on a local SSD. `make perf-compare` was used to generate the results.

As there are many results to sift through, here's a few key summary take-aways:

* If all you care about is cutting field values by column, `zeek-cut` does still perform the best. (Alas, that's all `zeek-cut` can do. :smiley:)

* The numerous input/output formats in `zq` are helpful for fitting into your legacy pipelines. However, ZNG performs the best of all `zq`-compatible formats, due to its binary/optimized nature. If you have logs in a non-ZNG format and expect to query them many times, a one-time pass through `zq` to convert them to ZNG format will save you significant time.

* Despite it having some CPU cost, the LZ4 compression that `zq` performs by default when outputting ZNG is shown to have a negligible user-perceptible performance impact. With this sample data, the LZ4-compressed ZNG is less than half the size of the uncompressed ZNG.

* Particularly when working in ZNG format & when simple analytics (counting, grouping) are in play, `zq` can significantly outperform `jq`. That said, `zq` does not (yet) include the full set of mathematical/other operations available in `jq`. If there's glaring functional omisssions that are limiting your use of `zq`, we welcome [contributions](../README.md#contributing).

# Results

The results below reflect performance as of `zq` commit `1813bdf8`.

### Output all events unmodified

|**<br>Tool**|**<br>Arguments**|**Input<br>Format**|**Output<br>Format**|**<br>Real**|**<br>User**|**<br>Sys**|
|:----------:|:---------------:|:-----------------:|:------------------:|-----------:|-----------:|----------:|
|`zq`|`*`|zeek|zeek|14.34|33.06|0.96|
|`zq`|`*`|zeek|zng|7.02|15.47|0.60|
|`zq`|`*`|zeek|zng-uncompressed|6.97|14.68|0.62|
|`zq`|`*`|zeek|tzng|10.64|26.32|0.76|
|`zq`|`*`|zeek|ndjson|51.86|74.82|1.28|
|`zq`|`*`|zng|zeek|12.72|19.41|0.58|
|`zq`|`*`|zng|zng|2.86|5.06|0.29|
|`zq`|`*`|zng|zng-uncompressed|2.70|4.39|0.30|
|`zq`|`*`|zng|tzng|9.42|13.79|0.50|
|`zq`|`*`|zng|ndjson|49.39|59.71|0.87|
|`zq`|`*`|zng-uncompressed|zeek|16.10|37.13|2.64|
|`zq`|`*`|zng-uncompressed|zng|11.34|19.61|2.47|
|`zq`|`*`|zng-uncompressed|zng-uncompressed|11.37|18.98|2.48|
|`zq`|`*`|zng-uncompressed|tzng|12.70|30.89|2.60|
|`zq`|`*`|zng-uncompressed|ndjson|50.48|75.55|2.73|
|`zq`|`*`|tzng|zeek|12.51|26.35|0.79|
|`zq`|`*`|tzng|zng|5.35|10.49|0.53|
|`zq`|`*`|tzng|zng-uncompressed|5.23|9.58|0.55|
|`zq`|`*`|tzng|tzng|9.58|20.83|0.64|
|`zq`|`*`|tzng|ndjson|48.35|66.00|1.15|
|`zq`|`*`|ndjson|zeek|61.83|96.45|1.57|
|`zq`|`*`|ndjson|zng|59.18|78.30|1.51|
|`zq`|`*`|ndjson|zng-uncompressed|57.79|75.90|1.31|
|`zq`|`*`|ndjson|tzng|59.03|87.89|1.44|
|`zq`|`*`|ndjson|ndjson|63.24|134.11|2.04|
|`zeek-cut`||zeek|zeek-cut|1.20|1.24|0.15|
|`jq`|`-c '.'`|ndjson|ndjson|35.58|39.28|1.84|

### Extract the field `ts`

|**<br>Tool**|**<br>Arguments**|**Input<br>Format**|**Output<br>Format**|**<br>Real**|**<br>User**|**<br>Sys**|
|:----------:|:---------------:|:-----------------:|:------------------:|-----------:|-----------:|----------:|
|`zq`|`cut ts`|zeek|zeek|6.46|15.45|0.56|
|`zq`|`cut ts`|zeek|zng|6.25|13.54|0.66|
|`zq`|`cut ts`|zeek|zng-uncompressed|6.32|13.70|0.62|
|`zq`|`cut ts`|zeek|tzng|6.45|15.24|0.58|
|`zq`|`cut ts`|zeek|ndjson|7.00|19.35|0.72|
|`zq`|`cut ts`|zng|zeek|3.14|6.54|0.32|
|`zq`|`cut ts`|zng|zng|2.89|4.78|0.25|
|`zq`|`cut ts`|zng|zng-uncompressed|2.89|4.71|0.23|
|`zq`|`cut ts`|zng|tzng|3.08|6.26|0.29|
|`zq`|`cut ts`|zng|ndjson|5.18|9.38|0.46|
|`zq`|`cut ts`|zng-uncompressed|zeek|11.20|19.96|2.22|
|`zq`|`cut ts`|zng-uncompressed|zng|10.99|17.97|2.47|
|`zq`|`cut ts`|zng-uncompressed|zng-uncompressed|11.07|18.19|2.31|
|`zq`|`cut ts`|zng-uncompressed|tzng|11.24|19.42|2.58|
|`zq`|`cut ts`|zng-uncompressed|ndjson|11.52|24.01|2.40|
|`zq`|`cut ts`|tzng|zeek|5.36|11.83|0.46|
|`zq`|`cut ts`|tzng|zng|5.14|9.89|0.50|
|`zq`|`cut ts`|tzng|zng-uncompressed|5.14|9.86|0.47|
|`zq`|`cut ts`|tzng|tzng|5.30|11.39|0.52|
|`zq`|`cut ts`|tzng|ndjson|6.08|16.15|0.55|
|`zq`|`cut ts`|ndjson|zeek|58.09|77.87|1.60|
|`zq`|`cut ts`|ndjson|zng|59.88|79.25|1.58|
|`zq`|`cut ts`|ndjson|zng-uncompressed|62.32|82.51|1.53|
|`zq`|`cut ts`|ndjson|tzng|60.05|80.63|1.41|
|`zq`|`cut ts`|ndjson|ndjson|61.17|85.59|1.57|
|`zeek-cut`|`ts`|zeek|zeek-cut|1.33|1.39|0.19|
|`jq`|`-c '. \| { ts }'`|ndjson|ndjson|20.61|23.18|1.32|

### Count all events

|**<br>Tool**|**<br>Arguments**|**Input<br>Format**|**Output<br>Format**|**<br>Real**|**<br>User**|**<br>Sys**|
|:----------:|:---------------:|:-----------------:|:------------------:|-----------:|-----------:|----------:|
|`zq`|`count()`|zeek|zeek|6.52|13.30|0.56|
|`zq`|`count()`|zeek|zng|6.83|13.98|0.56|
|`zq`|`count()`|zeek|zng-uncompressed|6.81|14.11|0.42|
|`zq`|`count()`|zeek|tzng|6.50|13.36|0.51|
|`zq`|`count()`|zeek|ndjson|6.44|13.18|0.53|
|`zq`|`count()`|zng|zeek|2.66|4.01|0.22|
|`zq`|`count()`|zng|zng|2.66|4.01|0.23|
|`zq`|`count()`|zng|zng-uncompressed|2.68|4.08|0.20|
|`zq`|`count()`|zng|tzng|2.70|4.12|0.16|
|`zq`|`count()`|zng|ndjson|2.68|4.06|0.19|
|`zq`|`count()`|zng-uncompressed|zeek|10.87|17.77|2.45|
|`zq`|`count()`|zng-uncompressed|zng|11.31|18.51|2.56|
|`zq`|`count()`|zng-uncompressed|zng-uncompressed|11.14|18.20|2.56|
|`zq`|`count()`|zng-uncompressed|tzng|11.21|18.27|2.56|
|`zq`|`count()`|zng-uncompressed|ndjson|11.18|18.10|2.70|
|`zq`|`count()`|tzng|zeek|5.25|9.31|0.43|
|`zq`|`count()`|tzng|zng|5.24|9.41|0.37|
|`zq`|`count()`|tzng|zng-uncompressed|5.27|9.42|0.38|
|`zq`|`count()`|tzng|tzng|5.36|9.68|0.34|
|`zq`|`count()`|tzng|ndjson|5.31|9.57|0.36|
|`zq`|`count()`|ndjson|zeek|61.28|79.94|1.31|
|`zq`|`count()`|ndjson|zng|62.63|81.52|1.44|
|`zq`|`count()`|ndjson|zng-uncompressed|63.57|83.07|1.27|
|`zq`|`count()`|ndjson|tzng|62.28|81.10|1.43|
|`zq`|`count()`|ndjson|ndjson|62.93|82.19|1.37|
|`jq`|`-c -s '. \| length'`|ndjson|ndjson|23.10|24.06|3.17|

### Count all events, grouped by the field `id.orig_h`

|**<br>Tool**|**<br>Arguments**|**Input<br>Format**|**Output<br>Format**|**<br>Real**|**<br>User**|**<br>Sys**|
|:----------:|:---------------:|:-----------------:|:------------------:|-----------:|-----------:|----------:|
|`zq`|`count() by id.orig_h`|zeek|zeek|6.92|14.96|0.56|
|`zq`|`count() by id.orig_h`|zeek|zng|6.70|14.43|0.49|
|`zq`|`count() by id.orig_h`|zeek|zng-uncompressed|6.67|14.24|0.56|
|`zq`|`count() by id.orig_h`|zeek|tzng|6.67|14.31|0.49|
|`zq`|`count() by id.orig_h`|zeek|ndjson|6.60|14.09|0.53|
|`zq`|`count() by id.orig_h`|zng|zeek|3.04|4.78|0.23|
|`zq`|`count() by id.orig_h`|zng|zng|3.02|4.70|0.25|
|`zq`|`count() by id.orig_h`|zng|zng-uncompressed|3.06|4.80|0.23|
|`zq`|`count() by id.orig_h`|zng|tzng|3.05|4.79|0.24|
|`zq`|`count() by id.orig_h`|zng|ndjson|3.05|4.80|0.20|
|`zq`|`count() by id.orig_h`|zng-uncompressed|zeek|11.78|19.03|2.81|
|`zq`|`count() by id.orig_h`|zng-uncompressed|zng|12.94|21.09|2.94|
|`zq`|`count() by id.orig_h`|zng-uncompressed|zng-uncompressed|13.53|22.10|3.02|
|`zq`|`count() by id.orig_h`|zng-uncompressed|tzng|13.65|22.27|2.98|
|`zq`|`count() by id.orig_h`|zng-uncompressed|ndjson|13.52|22.20|2.83|
|`zq`|`count() by id.orig_h`|tzng|zeek|5.79|10.99|0.41|
|`zq`|`count() by id.orig_h`|tzng|zng|5.85|11.16|0.33|
|`zq`|`count() by id.orig_h`|tzng|zng-uncompressed|5.83|11.07|0.43|
|`zq`|`count() by id.orig_h`|tzng|tzng|5.78|10.97|0.40|
|`zq`|`count() by id.orig_h`|tzng|ndjson|5.76|11.01|0.35|
|`zq`|`count() by id.orig_h`|ndjson|zeek|69.41|92.67|1.46|
|`zq`|`count() by id.orig_h`|ndjson|zng|67.72|90.28|1.59|
|`zq`|`count() by id.orig_h`|ndjson|zng-uncompressed|67.57|89.70|1.65|
|`zq`|`count() by id.orig_h`|ndjson|tzng|63.24|83.79|1.43|
|`zq`|`count() by id.orig_h`|ndjson|ndjson|59.09|78.34|1.24|
|`jq`|`-c -s 'group_by(."id.orig_h")[] \| length as $l \| .[0] \| .count = $l \| {count,"id.orig_h"}'`|ndjson|ndjson|31.79|32.29|3.19|

### Output all events with the field `id.resp_h` set to `52.85.83.116`

|**<br>Tool**|**<br>Arguments**|**Input<br>Format**|**Output<br>Format**|**<br>Real**|**<br>User**|**<br>Sys**|
|:----------:|:---------------:|:-----------------:|:------------------:|-----------:|-----------:|----------:|
|`zq`|`id.resp_h=52.85.83.116`|zeek|zeek|6.18|12.87|0.44|
|`zq`|`id.resp_h=52.85.83.116`|zeek|zng|6.16|12.79|0.47|
|`zq`|`id.resp_h=52.85.83.116`|zeek|zng-uncompressed|6.27|13.03|0.57|
|`zq`|`id.resp_h=52.85.83.116`|zeek|tzng|6.21|12.80|0.58|
|`zq`|`id.resp_h=52.85.83.116`|zeek|ndjson|6.14|12.68|0.56|
|`zq`|`id.resp_h=52.85.83.116`|zng|zeek|2.66|3.92|0.21|
|`zq`|`id.resp_h=52.85.83.116`|zng|zng|2.62|3.89|0.20|
|`zq`|`id.resp_h=52.85.83.116`|zng|zng-uncompressed|2.63|3.89|0.20|
|`zq`|`id.resp_h=52.85.83.116`|zng|tzng|2.67|3.98|0.21|
|`zq`|`id.resp_h=52.85.83.116`|zng|ndjson|2.72|4.11|0.17|
|`zq`|`id.resp_h=52.85.83.116`|zng-uncompressed|zeek|10.58|17.23|2.19|
|`zq`|`id.resp_h=52.85.83.116`|zng-uncompressed|zng|10.49|16.97|2.37|
|`zq`|`id.resp_h=52.85.83.116`|zng-uncompressed|zng-uncompressed|10.55|17.15|2.30|
|`zq`|`id.resp_h=52.85.83.116`|zng-uncompressed|tzng|10.47|16.90|2.35|
|`zq`|`id.resp_h=52.85.83.116`|zng-uncompressed|ndjson|10.48|16.95|2.32|
|`zq`|`id.resp_h=52.85.83.116`|tzng|zeek|4.96|8.95|0.43|
|`zq`|`id.resp_h=52.85.83.116`|tzng|zng|5.01|9.06|0.36|
|`zq`|`id.resp_h=52.85.83.116`|tzng|zng-uncompressed|5.02|9.09|0.35|
|`zq`|`id.resp_h=52.85.83.116`|tzng|tzng|5.17|9.49|0.31|
|`zq`|`id.resp_h=52.85.83.116`|tzng|ndjson|5.40|9.99|0.36|
|`zq`|`id.resp_h=52.85.83.116`|ndjson|zeek|58.41|76.16|1.31|
|`zq`|`id.resp_h=52.85.83.116`|ndjson|zng|58.29|75.91|1.37|
|`zq`|`id.resp_h=52.85.83.116`|ndjson|zng-uncompressed|58.25|75.85|1.32|
|`zq`|`id.resp_h=52.85.83.116`|ndjson|tzng|58.39|76.07|1.37|
|`zq`|`id.resp_h=52.85.83.116`|ndjson|ndjson|58.31|76.00|1.30|
|`jq`|`-c '. \| select(.["id.resp_h"]=="52.85.83.116")'`|ndjson|ndjson|17.69|20.30|1.11|

