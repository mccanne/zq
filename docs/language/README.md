# Zed Language

An ambitious goal of the Zed project is to offer a language
&mdash; the _Zed language_ &mdash;
that provides an easy learning curve and a gentle slope from simple keyword search
to log-search-style processing and ultimately to sophisticated, large-scale
warehouse-scale queries.  The language also embraces a rich set of type operators
based on the [Zed data model](../formats/zson.md) for data shaping
to provide flexible and easy ETL.

The simplest Zed program is perhaps a single word search, e.g.,
```
widget
```
This program searches the implied input for Zed records that
contain the string "widget".

> NOTE: we should clarify keyword search vs substring match.

As with the unix shell and legacy log search systems,
the Zed language embraces a _pipeline_ model where a source of data
is treated as a stream then one or more operators concatenated with
the `|` symbol transform, filter, and aggregate the stream, e.g.,
```
widget | price > 1000
```

That said, the Zed language is declarative and
the Zed compiler optimizes the data flow computation
&mdash; e.g., implementing a Zed program often differently than
the flow implied by the pipeline yet reaching the same result &mdash;
much as a modern SQL engine optimizes a declarative SQL query.

For example, the query from above is more efficiently implemented as
a boolean AND operation instead of two pipeline stages,
so the compiler is free to transform it to
```
widget and price > 1000
```
And since the "AND" syntax is optional (logical AND can be expressed as
concatenation), this query can also be expressed as
```
widget price > 1000
```

To facilitate both a programming-like model as well as an ad hoc search
experience, Zed has a canonical, long form that can be abbreviated
using syntax that supports an agile interactive query workflow.
For example, the canonical form of an aggregation uses the `summarize`
keyword, as in
```
summarize count() by id
```
but this can be abbreviated by dropping the keyword whereby the compiler then
uses the name of the aggregation function to resolve the ambiguity, e.g.,
as in the shorter form
```
count() by id
```
Similarly, the canonical form of a search expression is a `filter` operator
(and the "AND" operator is explicit in canonical form),
so the example from above would be written canonically as
```
filter widget and price > 1000
```
Boolean expressions can also appear as simple search filters and these various
operators composed in a simple to type and edit fashion:
```
widget price > 1000 | count() by color | count >= 10 | sort count
```
The canonical form of this more complex query is:
```
filter widget and price > 1000
  | summarize count() by color
  | filter count >= 10
  | sort count
```
To encourage adoption by the vast audience of users who know and love SQL,
a key goal of Zed is to support a superset of the data query language (DQL) portion
of ANSI SQL.  For example, the above query can also be written in Zed as
```
SELECT count(), color
WHERE widget AND price > 1000
GROUP BY color
HAVING count >= 10
ORDER BY count
```
i.e., this SQL expression is a subset of the Zed language.
Naturally, the SQL and Zed forms can be mixed and matched:
```
SELECT count(), color
WHERE widget AND price > 1000
GROUP BY color
  | count >= 10 | sort count
```
While this hybrid capability of Zed may seem questionable, our goal here
is to have the best of both worlds: the easy interactive workflow of Zed
combined with the ubiquity and familiarity of SQL.

And because the Zed data model
is based on a heterogenous sequence of arbitrarily typed semi-structured records,
the Zed language is often better fit here compared to SQL.  For example, an aggregation
that operates on heterogeneous data might look like this:
```
not srcip in 192.168.0.0/16
| summarize
    bytes := sum(src_bytes + dst_bytes),
    maxdur := max(duration),
    valid := and(status == "ok")
      by srcip, dstip
```
This query filters out records in with `srcip` in network 192.168
and computes three aggregations over all such records that have the `srcip` and `dstip`
fields where some record have a `status` field, other records
have a `duration` field and yet other records have
`src_bytes` and `dst_bytes` fields.  Because Zed is more relaxed than SQL,
you can throw together a bunch of related data of different types into a "data pool"
without having to define any upfront schemas
&mdash; let alone a schema per table &mdash;
thereby enabling easy-to-write queries over heterogenous pools of data.
Writing an equivalent SQL query for the different record types implied above
would require complicated table references, nested selects, and multi-way joins.

> NOTE that the SQL expression implementation is currently in prototype stage.
> If you try it out, you may run into problems and we'd love your
> feedback for where it breaks and how it can be improved.

## Data Sources

In the examples, above the data source is implied.  For example, the
`zed query` command takes a list of files and the concatenated files
are the implied input.  Likewise, in the Brim app, the UI selects a
data source and key range.

Data sources can also be explicitly specified using the `from` keyword.
Depending on the operating context, `from` make take a file path argument
relative to the local file system, an HTTP URL, an S3 URL, or in the
context of a Zed lake, the name of a data pool.

## Directed-acyclic Flow Graphs

While the examples above all illustrate a linear sequence of operations,
Zed programs can include multiple data sources and splitting operations
where multiple paths run in parallel and paths can be combined (in an
undefined order), merged (in a defined order) by one or more sort keys,
or joined using relational join logic.

Generally speaking, a flowgraph defines a directed acyclic graph (DAG) composed
of data sources and operator nodes.

A data path can be split with the `split` operator as in
```
from PoolOne | split (
  => op1 | op2 | ...
  => op1 | op2 | ...
) | merge ts | ...
```
Or multiple pools can be accessed and, for example, joined...
```
from (
  PoolOne => op1 | op2 | ... ;
  PoolTwo => op1 | op2 | ... ;
) | join on key=key | ...
```

XXX change this example to a flowgraph

![Example Zed 1](images/example-zed.png)

As is typical with pipelines, you can imagine the data flowing left-to-right
through this chain of processing elements, such that the output of each element
is the input to the next.  While Zed follows the common pattern seen in
other query languages where the pipeline begins with a search and further
processing is then performed on the isolated data, one of Zed's
strengths is that searches and expressions can appear in any order in the
pipeline.

![Example Zed 2](images/example-zed-operator-search.png)

The available pipeline elements are broadly categorized into:

* _[Searches](search-syntax/README.md)_ that isolate subsets of your data,
* _[Operators](operators/README.md)_ that transform or filter records,
* _[Expressions](expressions/README.md)_ for invoking functions or performing math and string processing on values,
* _[Aggregate Functions](aggregate-functions/README.md)_ that carry out running computations based on the values of fields in successive events, and
* _[Grouping](grouping/README.md)_ techniques to partition data based on field values.

To build effective queries, it is also important to become familiar with the
Zed _[Data Types](data-types/README.md)_.

Each of the sections hyperlinked above describes these elements of the language
in more detail. To make effective use of the materials, it is recommended to
first review the [Documentation Conventions](conventions/README.md). You will
likely also want to download a copy of the
[Sample Data](https://github.com/brimdata/zed-sample-data) so you can reproduce
the examples shown.
