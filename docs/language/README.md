# Zed Language

An ambitious goal of the Zed project is to offer a language --- the Zed language ---
that provides an easy learning curve and a gentle slope from simple keyword search
to log-search-style processing and ultimately to sophisticated, large-scale
warehouse-scale queries.  The language also embraces a rich set of type operators
based on the [Zed data model](../formats/zson.md) for data shaping
and for flexible and easy ETL.

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
the `|` symbol transform, filter, and aggregate the stream.

That said, the Zed language is declarative and
the Zed compiler optimizes the data flow computation --- e.g., implementing
a Zed program often differently than the flow implied by the pipeline
yet reaching the same result --- much as a modern
SQL engine optimizes a declarative SQL query.

To facilitate both a programming-like model as well as an ad hoc search
experience, the language has a canonical form that can be abbreviated
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
Boolean expressions can also appear as simple search filters and these various
operators composed in a simple to type and edit fashion:
```
widget | ipsrc in 192.168.0.0/16 | count() by id | count > 1000 | sort count
```
The canonical Zed form here would be:
```
filter match(widget)
  | filter ipsrc in 192.168.0.0/16
  | summarize count() by id |
  | filter count > 1000
  | sort count
```
To support adoption by the vast audience of users who know and love SQL,
a key goal of Zed is to support a superset of the data query language (DQL) portion
of ANSI SQL.  For example, the above query can also be written in Zed as
```
SELECT count(), id
WHERE match(hello) AND ipsrc in 128.32.0.0/16
GROUP BY id
HAVING count > 1000
ORDER BY count
```
and, of course, the SQL and Zed forms can be mixed and matched:
```
SELECT count(), id
WHERE match(widget) AND ipsrc in 192.168.0.0/16
GROUP BY id
| count > 1000 | sort count
```
While this hybrid capability of Zed is easy to question, the goal here
is to  have the best of both worlds: simple and easy interactive workflow UX
of Zed combined with the precision and familiarity of SQL.

Moreover, the Zed data model
is based on a heterogenous sequence of arbitrarily typed semi-structured records
and the Zed language is a better fit here compared to SQL.  For example, an aggregation
that operates on heterogeneous data might look like this:
```
bytes := sum(src_bytes + dst_bytes),
maxdur := max(duration),
valid := or(status != "ok")
     by ipsrc, ipdst
```
This computes the aggregation over all records that have the `ipsrc` and `ipdst`
fields where some record have a `status` field, other record
have a `duration` field and yet other record have
`src_bytes` and `dst_bytes` fields.  Because Zed is more relaxed than SQL,
you can throw together a bunch of related data of different types into a "data pool"
without having to define any upfront schemas --- let alone a schema per table ---
thereby enabling easy-to-write queries over heterogenous pools of data.
Writing a SQL query for the different record types here would require complicated
table references, nested selects, and joins.

> NOTE that the SQL expression implementation is currently in prototype stage.
> If you try it out, you may run into problems and we'd love your
> feedback for where it breaks and how it can be improved.

Here's a simple example query:

![Example Zed 1](images/example-zed.png)

As is typical with pipelines, you can imagine the data flowing left-to-right
through this chain of processing elements, such that the output of each element
is the input to the next. The example above follows a common pattern seen in
other query languages where the pipeline begins with a search and further
processing is then performed on the isolated data. However, one of Zed's
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
