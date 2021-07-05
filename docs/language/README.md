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
summarize count() by color
```
but this can be abbreviated by dropping the keyword whereby the compiler then
uses the name of the aggregation function to resolve the ambiguity, e.g.,
as in the shorter form
```
count() by color
```
Similarly, the canonical form of a search expression is a `filter` operator
(and the "AND" operator is explicit in canonical form),
so the example from above would be written canonically as
```
filter widget and price > 1000
```
Unlike typical log search systems, the Zed language operators are uniform:
you can specify an operator including keyword search terms, boolean predicates,
etc using the same syntax at any point in the pipeline.  For example,
the predicate `count >= 10` can simply be tacked onto the output of a
count aggregation using the filter from above and perhaps sorting
the final about by `count` in a simple to type and edit fashion:
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
or joined using relational join logic (currently only merge-based equijoin
is supported).

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
Similarly, data can be routed to different paths with replication
using `switch`:
```
from ... | switch (
  case color == "red" => op1 | op2 | ...
  case color == "blue" => op1 | op2 | ...
  case * => op1 | op2 | ...
) | ...
```

> TBD: (MOVE THIS TO AN ISSUE)
> In writing the switch example, I think we should unify the syntax with  
> split by dropping the "case" keyword and using semicolon for path termination.
> Also, we should have a form where you can switch on an expr and have a
> "default" keyword, e.g.,
>   switch color (
>     "red" => op1 | op2 | ...
>     "blue" => op1 | op2 | ...
>     default => op1 | op2 | ...
>   )

## Operators

XXX work in progress

Each operator performs a specific operation on a stream of records and
is identified by name.  The entire list of operators is documented
in the [Zed Operator Reference](operators/README.md).

For three important and commonly used set of operators, the operator name
is optional as the compiler can determine from syntax and context, which of the
unnamed operators is intended.
  This promotes an easy-to-type UX for these common use cases.
They include:
* _filter_ - drops all input that does no match a specified search expression
* _summarize_ - perform zero or more aggregations with optional group-by keys
* _put_ - add or modify fields to records

For example, the canonical form of
```
filter widget
| summarize count() by color
| put COLOR := to_upper(color)
```
can be abbreviated as
```
widget | count() by color | COLOR := to_upper(color)
```
as the compiler can tell from syntax and context that the three operators
are a filter, summarize, and put.

All other operators are explicitly named.

# TODO

Put here a list of all the operators with a link to each markdown file,
one per operator.

### Filter

The filter operator takes a
[search expression](search-syntax/README.md),
which consists of literal matches, glob matches, regular expression matches,
boolean expression predicates, or any of the above intermixed using
boolean logic (AND, OR, NOT), where the AND operator can be elided and replaced
with concatenation.

For literal matches, the literal is searched across all of the values in a
record including nested values.  For string literals, any substring of a larger
string field may match as well well as any field name.
For non-string literals, fields that are compatible with the literal's
type are searched for an exact

When searching for string literals in a search expression, quotes are optional
when the string is a simple identifier that does not conflict with Zed
reserved keywords.  This provides the look and feel of a search language
like email search or log search while being embedded with the much broader
Zed language and makes interactive searching more agile.

For example, this search expression combines various search idioms
```
widget or 123 or foo*bar or "hello, world" or 192.168.1.1 or /foo.*bar/
```
while this expression includes boolean predicates and logic
```
widget or 123 or count > 10 or not (color == "red" or color == "blue")
```

### Summarize

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
