# Zed/Zeek Data Type Compatibility

- [Introduction](#introduction)
- [Equivalent Types](#equivalent-types)
- [Example](#example)
- [Type-Specific Details](#type-specific-details)
  * [`double`](#double)
  * [`enum`](#enum)
  * [`port`](#port)
  * [`set`](#set)
  * [`string`](#string)
  * [`record`](#record)

## Introduction

As the Zed data model was in many ways inspired by the
[Zeek TSV log format](https://docs.zeek.org/en/master/log-formats.html#zeek-tsv-format-logs),
the rich Zed storage formats ([ZSON](../docs/formats/zson.md),
[ZNG](../docs/formats/zng.md), etc.) maintain comprehensive interoperability
with Zeek. When Zeek is configured to output its logs in
NDJSON format, much of the rich type information is lost in translation, but
this can be restored by following the guidance for [shaping Zeek NDJSON](Shaping-Zeek-NDJSON.md).
On the other hand, Zeek TSV can be converted to Zed storage formats and back to
Zeek TSV without any loss of information.

This document describes how the Zed type system is able to represent each of
the types that may appear in Zeek logs.

Tools like [`zq`](https://github.com/brimdata/zed) and
[Brim](https://github.com/brimdata/brim) maintain an internal Zed-typed
representation of any Zeek data that is read or imported. Therefore, knowing
the equivalent types will prove useful when performing operations in the
[Zed language](../docs/language/README.md) such as
[type casting](../docs/language/data-types#example) or looking at the data
when output as ZSON.

## Equivalent Types

The following table summarizes which Zed data type corresponds to each
[Zeek data type](https://docs.zeek.org/en/current/script-reference/types.html)
that may appear in a Zeek TSV log. While most types have a simple 1-to-1
mapping from Zeek to Zed and back to Zeek again, the sections linked from the
**Additional Detail** column describe cosmetic differences and other subtleties
applicable to handling certain types.

| Zeek Type  | Zed Type   | Additional Detail |
|------------|------------|-------------------|
| [`bool`](https://docs.zeek.org/en/current/script-reference/types.html#type-bool)         | [`bool`](../docs/formats/zson.md#33-primitive-values)     | |
| [`count`](https://docs.zeek.org/en/current/script-reference/types.html#type-count)       | [`uint64`](../docs/formats/zson.md#33-primitive-values)   | |
| [`int`](https://docs.zeek.org/en/current/script-reference/types.html#type-int)           | [`int64`](../docs/formats/zson.md#33-primitive-values)    | |
| [`double`](https://docs.zeek.org/en/current/script-reference/types.html#type-double)     | [`float64`](../docs/formats/zson.md#33-primitive-values)  | See [`double` details](#double) |
| [`time`](https://docs.zeek.org/en/current/script-reference/types.html#type-time)         | [`time`](../docs/formats/zson.md#33-primitive-values)     | |
| [`interval`](https://docs.zeek.org/en/current/script-reference/types.html#type-interval) | [`duration`](../docs/formats/zson.md#33-primitive-values) | |
| [`string`](https://docs.zeek.org/en/current/script-reference/types.html#type-string)     | [`bstring` or `string`](../docs/formats/zson.md#33-primitive-values) | See [`string` details](#string) |
| [`port`](https://docs.zeek.org/en/current/script-reference/types.html#type-port)         | [`uint16`](../docs/formats/zson.md#33-primitive-values)   | See [`port` details](#port) |
| [`addr`](https://docs.zeek.org/en/current/script-reference/types.html#type-addr)         | [`ip`](../docs/formats/zson.md#33-primitive-values)       | |
| [`subnet`](https://docs.zeek.org/en/current/script-reference/types.html#type-subnet)     | [`net`](../docs/formats/zson.md#33-primitive-values)      | |
| [`enum`](https://docs.zeek.org/en/current/script-reference/types.html#type-enum)         | [`string`](../docs/formats/zson.md#33-primitive-values)   | See [`enum` details](#enum) |
| [`set`](https://docs.zeek.org/en/current/script-reference/types.html#type-set)           | [`set`](../docs/formats/zson.md#343-set-value)       | See [`set` details](#set) |
| [`vector`](https://docs.zeek.org/en/current/script-reference/types.html#type-vector)     | [`array`](../docs/formats/zson.md#342-array-value)   | |
| [`record`](https://docs.zeek.org/en/current/script-reference/types.html#type-record)     | [`record`](../docs/formats/zson.md#341-record-value) | See [`record` details](#record) |

> **Note:** The [Zeek data type](https://docs.zeek.org/en/current/script-reference/types.html)
> page describes the types in the context of the
> [Zeek scripting language](https://docs.zeek.org/en/master/scripting/index.html).
> The Zeek types available in scripting are a superset of the data types that
> may appear in Zeek log files. The encodings of the types also differ in some
> ways between the two contexts. However, we link to this reference because
> there is no authoritative specification of the Zeek TSV log format.

## Example

The following example shows an input TSV log that includes each Zeek data type,
how it's output as ZSON by `zq`, and then how it's written back out again as a Zeek
log. You may find it helpful to refer to this example when reading the
[Type-Specific Details](#type-specific-details) sections.

```mdtest-input zeek_types.log
#separator \x09
#set_separator	,
#empty_field	(empty)
#unset_field	-
#fields	my_bool	my_count	my_int	my_double	my_time	my_interval	my_printable_string	my_bytes_string	my_port	my_addr	my_subnet	my_enum	my_set	my_vector	my_record.name	my_record.age
#types	bool	count	int	double	time	interval	string	string	port	addr	subnet	enum	set[string]	vector[string]	string	count
T	123	456	123.4560	1592502151.123456	123.456	smile\xf0\x9f\x98\x81smile	\x09\x07\x04	80	127.0.0.1	10.0.0.0/8	tcp	things,in,a,set	order,is,important	Jeanne	122
```

```mdtest-command
zq -Z zeek_types.log | tee zeek_types.zson
```

```mdtest-output
{
    my_bool: true,
    my_count: 123 (uint64),
    my_int: 456,
    my_double: 123.456,
    my_time: 2020-06-18T17:42:31.123456Z,
    my_interval: 2m3.456s,
    my_printable_string: "smile😁smile" (bstring),
    my_bytes_string: "\t\x07\x04" (bstring),
    my_port: 80 (port=(uint16)),
    my_addr: 127.0.0.1,
    my_subnet: 10.0.0.0/8,
    my_enum: "tcp" (=zenum),
    my_set: |[
        "a" (bstring),
        "in" (bstring),
        "set" (bstring),
        "things" (bstring)
    ]| (=0),
    my_vector: [
        "order" (bstring),
        "is" (bstring),
        "important" (bstring)
    ] (=1),
    my_record: {
        name: "Jeanne" (bstring),
        age: 122 (uint64)
    } (=2)
} (=3)
```

```mdtest-command
zq -f zeek zeek_types.zson
```

```mdtest-output
#separator \x09
#set_separator	,
#empty_field	(empty)
#unset_field	-
#fields	my_bool	my_count	my_int	my_double	my_time	my_interval	my_printable_string	my_bytes_string	my_port	my_addr	my_subnet	my_enum	my_set	my_vector	my_record.name	my_record.age
#types	bool	count	int	double	time	interval	string	string	port	addr	subnet	enum	set[string]	vector[string]	string	count
T	123	456	123.456	1592502151.123456	123.456	smile\xf0\x9f\x98\x81smile	\x09\x07\x04	80	127.0.0.1	10.0.0.0/8	tcp	a,in,set,things	order,is,important	Jeanne	122
```

## Type-Specific Details

As `zq` acts as a reference implementation for Zed storage formats such as
ZSON and ZNG, it's helpful to understand how it reads the following Zeek data
types into readable text equivalents in the ZSON format, then writes them back
out again in the Zeek TSV log format. Other implementations of the Zed storage
formats (should they exist) may handle these differently.

Multiple Zeek types discussed below are represented via a
[type definition](../docs/formats/zson.md#25-type-definitions) to one of Zed's
[primitive types](../docs/formats/zson.md#33-primitive-values). The Zed type
definitions maintain the history of the field's original Zeek type name
such that `zq` may restore it if the field is later output in
Zeek format. Knowledge of its original Zeek type may also enable special 
operations in Zed that are unique to values known to have originated as a
specific Zeek type, though no such operations are currently implemented in
`zq`.

### `double`

As they do not affect accuracy, "trailing zero" decimal digits on Zeek `double`
values will _not_ be preserved when they are formatted into a string, such as
via the ZSON/Zeek/table output options in `zq` (e.g., `123.4560` becomes
`123.456`).

### `enum`

As they're encountered in common programming languages, enum variables
typically hold one of a set of predefined values. While this is
how Zeek's `enum` type behaves inside the Zeek scripting language,
when the `enum` type is output in a Zeek log, the log does not communicate
any such set of "allowed" values as they were originally defined. Therefore,
these values are represented with a ZSON type name bound to the Zed `string`
type. See the text above regarding [type definitions](#type-specific-details)
for more details.


### `port`

The numeric values that appear in Zeek logs under this type are represented
in ZSON with a type name of `port` bound to the `uint16` type. See the text
above regarding [type names](#type-specific-details) for more details.

### `set`

Because order within sets is not significant, no attempt is made to maintain
the order of `set` elements as they originally appeared in a Zeek log.

### `string`

Zeek's `string` data type is complicated by its ability to hold printable ASCII
and UTF-8 as well as arbitrary unprintable bytes represented as `\x` escapes.
Because such binary data may need to legitimately be captured (e.g. to record
the symptoms of DNS exfiltration), it's helpful that Zeek has a mechanism to
log it. Unfortunately, Zeek's use of the single `string` type for these
multiple uses leaves out important details about the intended interpretation
and presentation of the bytes that make up the value. For instance, one Zeek
`string` field may hold arbitrary network data that _coincidentally_ sometimes
form byte sequences that could be interpreted as printable UTF-8, but they are
_not_ intended to be read or presented as such. Meanwhile, another Zeek
`string` field may be populated such that it will _only_ ever contain printable
UTF-8. These details are currently only captured within the Zeek source code
itself that defines how these values are generated.

Zed includes a [primitive type](../docs/formats/zson.md#33-primitive-values)
called `bytes` that's suited to storing the former "always binary" case and a
`string` type for the latter "always printable" case. However, Zeek logs do
not currently communicate details that would allow an implementation to know
which Zeek `string` fields to store as which of these two Zed data types.
Therefore the Zed `bstring` type is typically used to hold values that are
read from Zeek `string`-type fields.

One exception to this is Zeek's `_path` field. As it's a standard field that's
known to be populated by Zeek's logging system (or populated by `zq` when
reading some [Zeek JSON data](README.md#type-definition-structure--importance-of-_path))
`zq` currently stores the `_path` field using Zed's `string` type.

If Zeek were to provide an option to output logs directly in one or more of
Zed's richer storage storage formats, this would create an opportunity to
assign the appropriate Zed `bytes` or `string` type at the point of origin,
depending on what's known about how the field's value is intended to be
populated and used.

### `record`

Zeek's `record` type is unique in that every Zeek log line effectively _is_ a
record, with its schema defined via the `#fields` and `#types` directives in
the headers of each log file. The word "record" never appears explicitly in
the schema definition in Zeek logs.

Embedded records also subtly appear within Zeek log lines in the form of
dot-separated field names. A common example in Zeek is the
[`id`](https://docs.zeek.org/en/current/scripts/base/init-bare.zeek.html#type-conn_id)
record, which captures the source and destination IP addresses and ports for a
network connection as fields `id.orig_h`, `id.orig_p`, `id.resp_h`, and
`id.resp_p`. When reading such fields into their Zed equivalent, `zq` restores
the hierarchical nature of the record as it originally existed inside of Zeek
itself before it was output by its logging system. This enables operations in
Zed that refer to the record at a higher level but affect all values lower
down in the record hierarchy.

Revisiting the data from our example, we can output all fields within
`my_record` via this Zed operation:

```mdtest-command
zq -f zeek 'cut my_record' zeek_types.zson
```

```mdtest-output
#separator \x09
#set_separator	,
#empty_field	(empty)
#unset_field	-
#fields	my_record.name	my_record.age
#types	string	count
Jeanne	122
```
