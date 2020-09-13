# ZNG Specification

> ### Note: This specification is ALPHA and a work in progress.
> [Zq](https://github.com/brimsec/zq/blob/master/README.md)'s
> implementation of ZNG is tracking this spec and as it changes,
> the zq output format is subject to change.  In this branch,
> zq attempts to implement everything herein excepting:
>
> * the `bytes` type is not yet implemented,
> * the `enum` type is not yet implemented,
> * only streams of `record` types (which may consist of any combination of
>   other implemented types) may currently be expressed in value messages.
>
> Also, we are contemplating reducing the number of [primitive types](#5-primitive-types), e.g.,
> the number of variations in integer types.

* [1. Introduction](#1-introduction)
* [2. The ZNG Data Model](#2-the-zng-data-model)
* [3. ZNG Binary Format (ZNG)](#3-zng-binary-format-zng)
  + [3.1 Control Messages](#31-control-messages)
    - [3.1.1 Typedefs](#311-typedefs)
      - [3.1.1.1 Record Typedef](#3111-record-typedef)
      - [3.1.1.2 Array Typedef](#3112-array-typedef)
      - [3.1.1.3 Set Typedef](#3113-set-typedef)
      - [3.1.1.4 Union Typedef](#3114-union-typedef)
      - [3.1.1.5 Alias Typedef](#3115-alias-typedef)
    - [3.1.2 End-of-Stream Markers](#312-end-of-stream-markers)
    - [3.1.3 Compressed Value Message Block](#313-compressed-value-message-block)
  + [3.2 Value Messages](#32-value-messages)
* [4. ZNG Text Format (TZNG)](#4-zng-text-format-tzng)
  + [4.1 Control Messages](#41-control-messages)
    - [4.1.1 Type Binding](#411-type-binding)
    - [4.1.2 Type Alias](#412-type-alias)
    - [4.1.3 Application-Specific Payload](#413-application-specific-payload)
  + [4.2 Type Grammar](#42-type-grammar)
  + [4.3 Values](#43-values)
    - [4.3.1 Character Escape Rules](#431-character-escape-rules)
    - [4.3.2 Value Syntax](#432-value-syntax)
  + [4.4 Examples](#44-examples)
* [5. Primitive Types](#5-primitive-types)
* [Appendix A. Related Links](#appendix-a-related-links)

## 1. Introduction

ZNG is a format for structured data values, ideally suited for streams
of heterogeneously typed records, e.g., structured logs, where filtering and
analytics may be applied to a stream in parts without having to fully deserialize
every value.

ZNG has a binary form called _ZNG_ as well as text form called _TZNG_,
comprising a sequence of newline-terminated UTF-8 strings.

ZNG is richly typed and thinner on the wire than JSON.
ZNG strikes a balance between the narrowly typed but flexible
[newline-delimited JSON (NDJSON)](http://ndjson.org/) format and
a more structured approach like [Apache Avro](https://avro.apache.org).
Like NDJSON, the TZNG text format represents a sequence of data objects
that can be parsed line by line.

ZNG is type rich and embeds all type information in the stream while having a
binary serialization format that allows "lazy parsing" of fields such that
only the fields of interest in a stream need to be deserialized and interpreted.
Unlike Avro, ZNG embeds its schemas in the data stream and thereby admits
an efficient multiplexing of heterogeneous data types by prepending to each
data value a simple integer identifier to reference its type.

ZNG requires no external schema definitions as its type system
constructs schemas on the fly from within the stream using composable,
dynamic type definitions.  Given this, there is no need for
a schema registry service, though ZNG can be readily adapted to systems like
[Apache Kafka](https://kafka.apache.org/) which utilize such registries,
by having a connector translate the schemas implied in the
ZNG stream into registered schemas and vice versa.

ZNG is more expressive than JSON in that any JSON input
can be mapped onto ZNG and recovered by decoding
that ZNG back into JSON, but the converse is not true.

The ZNG design was motivated by and [is compatible with](./zeek-compat.md) the
[Zeek log format](https://docs.zeek.org/en/stable/examples/logs/).
As far as we know, the Zeek log format pioneered the concept of
embedding the schema of the log lines within the log file itself using
meta-records, and ZNG merely modernizes this original approach.

The [`zq`](https://github.com/brimsec/zq) command-line tool provides a
reference implementation of ZNG as it's described here, including the type
system, error handling, etc., barring the exceptions
described in the [alpha notice](#note-this-specification-is-alpha-and-a-work-in-progress)
at the top of this specification.

## 2. The ZNG Data Model

ZNG encodes a sequence of one or more typed data values to comprise a stream.
The stream of values is interleaved with control messages
that provide type definitions and other metadata.  The type of
a particular data value is specified by its "type identifier", or type ID,
which is an integer representing either a "primitive type" or a
"container type".

The ZNG type system comprises the standard set of primitive types like integers,
floating point, strings, byte arrays, etc. as well as container types
like records, arrays, and sets arranged from the primitive types.

For example, a TZNG stream representing the single string "hello world"
might look like this:
```
#35:string
35:hello, world
```
Here, the first line binds a tag `35` to the ZNG `string` data type
and the second line references that tag to specify a value of the `string`
type.

ZNG gets more interesting when different data types are interleaved in the
stream.  For example, consider this TZNG stream:
```
#35:string
35:hello, world
#36:int64
36:42
35:there's a fly in my soup!
35:no, there isn't.
36:3
```
Here the tag `36` now binds to one of ZNG's integer types. This encoding
represents the sequence of values that could be expressed in JSON as
```
"hello, world"
42
"there's a fly in my soup!"
"no, there isn't."
3
```
ZNG streams often comprise a sequence of records, which works well to
provide an efficient representation of structured logs. In this case, a new
type defines the schema for each distinct record. For example, the following
shows type bindings and values in TZNG for Zeek's `weird` and `ftp`
events:

```
#24:record[_path:string,ts:time,uid:bstring,id:record[orig_h:ip,orig_p:port,resp_h:ip,resp_p:port],name:bstring,addl:bstring,notice:bool,peer:bstring]
24:[weird;1521911720.600843;C1zOivgBT6dBmknqk;[10.47.1.152;49562;23.217.103.245;80;]TCP_ack_underflow_or_misorder;-;F;zeek;]
#25:record[_path:string,ts:time,uid:bstring,id:record[orig_h:ip,orig_p:port,resp_h:ip,resp_p:port],user:bstring,password:bstring,command:bstring,arg:bstring,mime_type:bstring,file_size:uint64,reply_code:uint64,reply_msg:bstring,data_channel:record[passive:bool,orig_h:ip,resp_h:ip,resp_p:port],fuid:bstring]
25:[ftp;1521911724.699488;ChkumY1k35TmZFL0V3;[10.164.94.120;45905;10.47.27.80;21;]anonymous;nessus@nessus.org;PASV;-;-;-;227;Entering Passive Mode (172,20,0,80,200,63).;[T;10.164.94.120;172.20.0.80;51263;]-;]
```
Note that the value encoding need not refer to the field names and types as
both are completely captured by the type definition. Values merely encode the
value information consistent with the referenced type.

## 3. ZNG Binary Format (ZNG)

The ZNG binary format is based on machine-readable data types with an
encoding methodology inspired by Avro and
[Protocol Buffers](https://developers.google.com/protocol-buffers).

A ZNG stream comprises a sequence of interleaved control messages and value messages
that are serialized into a stream of bytes.

Each message is prefixed with a single-byte header code.  The upper bit of
the header code indicates whether the message is a control message (1)
or a value message (0).

### 3.1 Control Messages

The lower 7 bits of a control header byte define the control code.
Control codes 0 through 6 are reserved for ZNG:

| Code | Message Type                   |
|------|--------------------------------|
| `0`  | record definition              |
| `1`  | array definition               |
| `2`  | set definition                 |
| `3`  | union definition               |
| `4`  | type alias                     |
| `5`  | end-of-stream                  |
| `6`  | compressed value message block |

All other control codes are available to higher-layer protocols to carry
application-specific payloads embedded in the ZNG stream.

Any such application-specific payloads not known by
a ZNG data receiver shall be ignored.

The body of an application-specific control message is any UTF-8 string.
These payloads are guaranteed to be preserved
in order within the stream and presented to higher layer components through
any ZNG streaming API.  In this way, senders and receivers of ZNG can embed
protocol directives as ZNG control payloads rather than defining additional
encapsulating protocols.

### 3.1.1 Typedefs

Following a header byte of 0x80-0x83 is a "typedef".  A typedef binds
"the next available" integer type ID to a type encoding.  Type IDs
begin at the value 23 and increase by one for each typedef. These bindings
are scoped to the stream in which the typedef occurs.

Type IDs for the "primitive types" need not be defined with typedefs and
are predefined with the IDs shown in the [Primitive Types](#5-primitive-types) table.

A typedef is encoded as a single byte indicating the container type ID followed by
the type encoding.  This creates a binding between the implied type ID
(i.e., 23 plus the count of all previous typedefs in the stream) and the new
type definition.

The type ID is encoded as a `uvarint`, an encoding used throughout the ZNG format.

> Inspired by Protocol Buffers,
> a `uvarint` is an unsigned, variable-length integer encoded as a sequence of
> bytes consisting of N-1 bytes with bit 7 clear and the Nth byte with bit 7 set,
> whose value is the base-128 number composed of the digits defined by the lower
> 7 bits of each byte from least-significant digit (byte 0) to
> most-significant digit (byte N-1).

#### 3.1.1.1 Record Typedef

A record typedef creates a new type ID equal to the next stream type ID
with the following structure:
```
----------------------------------------------------------
|0x80|<nfields>|<field1><type-id-1><field2><type-id-2>...|
----------------------------------------------------------
```
Record types consist of an ordered set of columns where each column consists of
a name and a typed value.  Unlike JSON, the ordering of the columns is significant
and must be preserved through any APIs that consume, process, and emit ZNG records.

A record type is encoded as a count of fields, i.e., `<nfields>` from above,
followed by the field definitions,
where a field definition is a field name followed by a type ID, i.e.,
`<field1>` followed by `<type-id-1>` etc. as indicated above.

The field names in a record must be unique.

The `<nfields>` is encoded as a `uvarint`.

The field name is encoded as a UTF-8 string defining a "ZNG identifier".
The UTF-8 string
is further encoded as a "counted string", which is the `uvarint` encoding
of the length of the string followed by that many bytes of UTF-8 encoded
string data.

N.B.: The rules for ZNG identifiers follow the same rules as
[JavaScript identifiers](https://tc39.es/ecma262/#prod-IdentifierName).

The type ID follows the field name and is encoded as a `uvarint`.

#### 3.1.1.2 Array Typedef

An array type is encoded as simply the type code of the elements of
the array encoded as a `uvarint`:
```
----------------
|0x81|<type-id>|
----------------
```

#### 3.1.1.3 Set Typedef

A set type is encoded as a type count followed by the type ID of the
elements of the set, encoded as a `uvarint`:
```
----------------
|0x82|<type-id>|
----------------
```

a container  primitive type ID.`<type-id>` may correspond to any valid zng type.

#### 3.1.1.4 Union Typedef

A union typedef creates a new type ID equal to the next stream type ID
with the following structure:
```
-----------------------------------------
|0x83|<ntypes>|<type-id-1><type-id-2>...|
-----------------------------------------
```
A union type consists of an ordered set of types
encoded as a count of the number of types, i.e., `<ntypes>` from above,
followed by the type IDs comprising the types of the union.
The type IDs of a union must be unique.

The `<ntypes>` and the type IDs are all encoded as `uvarint`.

`<ntypes>` cannot be 0.

#### 3.1.1.5 Alias Typedef

A type alias defines a new type ID that binds a new type name
to a previously existing type ID.  This is useful for systems like Zeek,
where there are customary type names that are well-known to users of the
Zeek system and are easily mapped onto a ZNG type having a different name.
By encoding the aliases in the format, there is no need to configure mapping
information across different systems using the format, as the type aliases
are communicated to the consumer of a ZNG stream.

A type alias is encoded as follows:
```
----------------------
|0x84|<name><type-id>|
----------------------
```
where `<name>` is an identifier representing the new type name with a new type ID
allocated as the next available type ID in the stream that refers to the
existing type ID ``<type-id>``.  ``<type-id>`` is encoded as a `uvarint` and `<name>`
is encoded as a `uvarint` representing the length of the name in bytes,
followed by that many bytes of UTF-8 string.

It is an error to define an alias that has the same name as a primitive type.
It is also an error to redefine a previously defined alias with a
type that differs from the original definition.

### 3.1.2 End-of-Stream Markers

A ZNG stream must be terminated by an end-of-stream marker.
A new ZNG stream may begin immediately after an end-of-stream marker.
Each such stream has its own, independent type context.

In this way, the concatenation of ZNG streams (or ZNG files containing
ZNG streams) results in a valid ZNG data sequence.

For example, a large ZNG file can be arranged into multiple, smaller streams
to facilitate random access at stream boundaries.
This benefit comes at the cost of some additional overhead --
the space consumed by stream boundary markers and repeated type definitions.
Choosing an appropriate stream size that balances this overhead with the
benefit of enabling random access is left up to implementations.

End-of-stream markers are also useful in the context of sending ZNG over Kafka,
as a receiver can easily resynchronize with a live Kafka topic by
discarding incomplete messages until a message is found that is terminated
by an end-of-stream marker (presuming the sender implementation aligns
the ZNG messages on Kafka message boundaries).

A end-of-stream marker is encoded as follows:
```
------
|0x85|
------
```

After this marker, all previously read
typedefs are invalidated and the "next available type ID" is reset to
the initial value of 23.  To represent subsequent records that use a
previously defined type, the appropriate typedef control code must
be re-emitted
(and note that the typedef may now be assigned a different ID).

### 3.1.3 Compressed Value Message Block

Following a header byte of 0x86 is a compressed value message block.
Such a block comprises a compressed sequence of value messages.  The
sequence must not include control messages.

A compressed value message block is encoded as follows:
```
------------------------------------------------------------------------------
|0x86|<format><uncompressed-length>|<compressed-length>|<compressed-messages>|
------------------------------------------------------------------------------

```
where
* `<format>`, a `uvarint`, identifies the algorthim used to compress the
  message sequence
* `<uncompressed-length>`, a `uvarint`, is the length in bytes of the
  uncompressed message sequence
* `<compressed-length>`, a `uvarint`, is the length in bytes of `<compressed-messages>`
* `<compressed-messages>` is the compressed value message sequence

Values for `<format>` are defined in the
[ZNG compression format specification](./compression-spec.md).

### 3.2 Value Messages

Following a header byte with bit 7 zero is a `typed value`
with a `uvarint7` encoding its length.

> A `uvarint7` is the same as a `uvarint` except only 7 bits instead of 8
> are available in the first byte.  Its value is equal to the lower 6-bits if bit 6
> of the first byte is 1; otherwise it is that value plus the value of the
> subsequent `uvarint` times 64.

A `typed value` is encoded as either a `uvarint7` (in a top-level value message)
or `uvarint` (for any other values)
encoding the length in bytes of the type ID and value followed by
the body of the typed value comprising that many bytes.
Within the body of the typed value,
the type ID is encoded as a `uvarint` and the value is encoded
as a byte array whose length is equal to the body length less the
length in bytes of the type ID.
```
------------------------
|uvarint7|type-id|value|
------------------------
```

It is an error for a value to reference a type ID that has not been previously
defined by a typedef scoped to the stream in which the value appears.

A typed value with a `value` of length `N` is interpreted as described in the
[Primitive Types](#5-primitive-types) table.

All multi-byte sequences representing machine words are serialized in
little-endian format.

> Note: The `bstring` type is an unusual type representing a hybrid type
> mixing a UTF-8 string with embedded binary data.  This type is
> useful in systems like Zeek where data is pulled off the network
> while expecting a string, but there can be embedded binary data due to
> bugs, malicious attacks, etc.  It is up to the receiver to determine
> with out-of-band information or inference whether the data is ultimately
> arbitrary binary data or a valid UTF-8 string.

A union value is encoded as a container with two elements. The first
element is the `uvarint` encoding of the index determining the type of
the value in reference to the union type, and the second element is
the value encoded according to that type.

Array, set, and record types are variable length and are encoded
as a sequence of elements:

| Type     |          Value                       |
|----------|--------------------------------------|
| `array`  | concatenation of elements            |
| `set`    | normalized concatenation of elements |
| `record` | concatenation of elements            |

Since N, the byte length of any of these container values, is known,
there is no need to encode a count of the
elements present.  Also, since the type ID is implied by the typedef
of any container type, each value is encoded without its type ID.

The concatenation of elements is encoded as a sequence of "tag-counted" values.
A tag carries both the length information of the corresponding value as well
a "container bit" to differentiate between primitive values and container values
without having to refer to the implied type.  This admits an efficient implementation
for traversing the values, inclusive of recursive traversal of container values,
whereby the inner loop need not consult and interpret the type ID of each element.

The tag encodes the length N of the value and indicates whether
it is a primitive value or a container value.
The length is offset by 1 whereby length of 0 represents an unset value
analogous to null in JSON.
The container bit is 1 for container values and 0 for primitive values.
The tag is defined as
```
2*(N+1) + the container bit
```
and is encoded as a `uvarint`.

For example, tag value 0 is an unset primitive value and tag value 1
is an unset container value.  Tag value 2 is a length zero primitive
value, e.g., it could represent empty string.  Tag value 3 is a length
zero container, such as an empty array or record with no fields.  Tag
value 4 is a length 1 primitive value, e.g., it would represent the
boolean "true" if followed by byte value 1 in the context of type ID 0
(i.e., the type ID for boolean).

Following the tag encoding is the value encoded in N bytes as described above.

For sets, the concatenation of elements must be normalized so that the
sequence of bytes encoding each element's tag-counted value is
lexicographically greater than that of the preceding element.

## 4. ZNG Text Format (TZNG)

The ZNG text format is a human-readable form that follows directly from the ZNG
binary format.  A TZNG file/stream is encoded with UTF-8.
All subsequent references to characters and strings in this section refer to
the Unicode code points that result when the stream is decoded.
If a TZNG stream includes data that is not valid UTF-8, the stream is invalid.

A stream of control messages and values messages is represented
as a sequence of lines each terminated by a newline.
Any newlines embedded in string-typed values must be escaped,
i.e., via `\u{0a}` or `\x0a`.

A line that begins with `#` is a control message and all other lines
are values.

### 4.1 Control Messages

TZNG control messages have one of three forms defined below.

Any line beginning with `#` that does not conform with the syntax described here
is an error.
When errors are encountered parsing TZNG, an implementation should return a
corresponding error and allow TZNG parsing to proceed if desired.

### 4.1.1 Type Binding

A TZNG type binding has the following form:
```
#<type-tag>:<type-string>
```
Here, `<type-tag>` is a string decimal integer and `<type-string>`
is a string defining a type (`<type>`) according to the [TZNG type grammar](#42-type-grammar). They create
a binding between the indicated tag and the indicated type.

### 4.1.2 Type Alias

A TZNG type alias has the following form:
```
#<type-name>=<type-string>
```
Here, `<type-name>` is an identifier and `<type-string>`
is a string defining a type (`<type>`) according to the [TZNG type grammar](#42-type-grammar). They create a
binding between the indicated tag and the indicated type.
This form defines an alias mapping the identifier to the indicated type.
`<type-name>` is an identifier with semantics as defined in [Section 3.1.1.5](#3115-alias-typedef).


### 4.1.3 Application-Specific Payload

A TZNG application-specific payload has the following form:
```
#!<control-code>:<payload>
```
Here, `<control-code>` is a decimal integer in the range 6-127 and `<payload>`
is any UTF-8 string with escaped newlines.

### 4.2 Type Grammar

Given the above textual definitions and the underlying ZNG specification, a
grammar describing the textual type encodings is:
```
<stype> := bool | byte | int16 | uint16 | int32 | uint32 | int64 | uint64 | float64
         | string | bytes | bstring | enum | ip | port | net | time | duration | null
         | <alias-name>

<ctype> := array [ <stype> ]
         | union [ <stype-list> ]
         | set [ <type> ]
         | record [ <columns> ]


<type> := <stype> | <ctype>

<stype-list> := <stype>
              | <stype-list> , <stype>

<columns> := <column>
           | <columns> , <column>

<column> := <id> : <type>

<alias-name> := <id>

<id> := <id-start> <id-continue>*

<id-start> := [A-Za-z_$]

<id-continue> := <id-start> | [0-9]
```

### 4.3 Values

A TZNG value is encoded on a line as a typed value, which is encoded as
an integer type code followed by `:`, which is in turn followed
by a value encoding.

Here is a pseudo-grammar for typed values:
```
<typed-value> := <tag> : <elem>
<tag> :=  0
        | [1-9][0-9]*
<elem> :=
          <terminal>
          <tag> : <terminal>
        | [ <list-elem>* ]
<list-elem> := <elem> ;
<terminal> := <char>*
```

A terminal value is encoded as a string of characters terminated
by a semicolon (which must be escaped if it appears in a string-typed value).
If the terminal value is of a union type, it is prefixed with the index of the value type in reference to the union type and a colon.

Container values (i.e., sets, arrays, or records) are encoded as
* an open bracket,
* zero or more encoded values terminated with semicolon, and
* a close bracket.

Any value can be specified as "unset" with the ASCII character `-`.
This is typically used to represent columns of records where not all
columns have been set in a given record value, though any type can be
validly unset.  A value that is not to be interpreted as "unset"
but is the single-character string `-`, must be escaped (e.g., `\x2d`).

Note that this syntax can be scanned and parsed independent of the
actual type definition indicated by the descriptor.  It is a semantic error
if the parsed value does not match the indicated type in terms of number and
sub-structure of value elements present and their interpretation as a valid
string of the specified type.

### 4.3.1 Character Escape Rules

Any Unicode code point may be represented in a `string` value using
the same `\u` syntax as JavaScript.  Specifically:
* The sequence `\uhhhh` where each `h` is a hexadecimal digit represents
  the Unicode code point corresponding to the given
  4-digit (hexadecimal) number, or:
* `\u{h*}` where there are from 1 to 6 hexadecimal digits inside the
  brackets represents the Unicode code point corresponding to the given
  hexadecimal number.

`\u` followed by anything that does not conform to the above syntax
is not a valid escape sequence.
The behavior of an implementation that encounters such
invalid sequences in a `string` type is undefined.

Any character in a `bstring` value may be escaped from the TZNG formatting rules
using the hex escape syntax, i.e., `\xhh` where `h` is a hexadecimal digit.
This allows binary data that does not conform to a valid UTF-8 character encoding
to be embedded in the `bstring` data type.
`\x` followed by anything other than two hexadecimal digits is not a valid
escape sequence. The behavior of an implementation that encounters such
invalid sequences in a `bstring` type is undefined.
Additionally, the backslash character itself (U+3B) may be represented
by a sequence of two consecutive backslash characters.  In other words,
the bstrings `\\` and `\x3b` are equivalent and both represent a single
backslash character.

These special characters must be escaped if they appear within a
`string` or `bstring` type: `;`, `\`, newline (Unicode U+0A).
These characters are invalid in all other data types.

Likewise, these characters must be escaped if they appear as the first character
of a value:
```
[ ]
```
In addition, `-` must be escaped if representing the single ASCII byte equal
to `-` as opposed to representing an unset value.

### 4.3.2 Value Syntax

Each UTF-8 string field parsed from a value line is interpreted according to the
type descriptor of the line using the formats shown in the
[Primitive Types](#5-primitive-types) table.

## 4.4 Examples

Here are some simple examples to get the gist of the ZNG text format.

Primitive types look like this:
```
bool
string
int64
```
Container types look like this:
```
#0:array[int64]
#1:set[bool,string]
#2:record[x:float64,y:float64]
```
Container types can be embedded in other container types by referencing
an earlier-defined type alias:
```
#REC=record[a:string,b:string,c:int64]
#SET=set[string]
#99:record[v:REC,s:SET,r:REC,s2:SET]
```
This TZNG defines a tag for the primitive string type and defines a record
and references the types accordingly in three values;
```
#0:string
#1:record[a:string,b:string]
0:hello, world;
1:[hello;world;]
0:this is a semicolon: \x3b;
```
which represents a stream of the three values, that could be expressed in JSON
as
```
"hello, world"
{"a": "hello", "b": "world"}
"this is a semicolon: ;"
```
Note that the tag integers occupy their own numeric space independent of
any underlying ZNG type IDs.

The semicolon terminator is important.  Consider this TZNG depicting
sets of strings:
```
#0:set[string]
0:[hello,world;]
0:[hello;world;]
0:[]
0:[;]
```
In this example:
* the first value is a `set` of one `string`
* the second value is a `set` of two `string` values, `hello` and `world`,
* the third value is an empty `set`, and
* the fourth value is a `set` containing one `string` of zero length.

In this way, an empty `set` and a `set` containing only a zero-length `string` can be distinguished.

This scheme allows containers to be embedded in containers, e.g., a
`record` inside of a `record` like this:
```
#LL:record[compass:string,degree:float64]
#26:record[city:string,lat:LL,long:LL]
26:[NYC;[N;40.7128;][W;74.0060;]]
```
An unset value indicates a field of a `record` that wasn't set by the encoder:
```
26:[North Pole;[N;90;]-;]
```
e.g., the North Pole has a latitude but no meaningful longitude.

## 5. Primitive Types

For each ZNG primitive type, the following table describes:
* The predefined ID, which need not be defined in [ZNG Typedefs](#311-typedefs)
* How a typed `value` of length `N` is interpreted in a [ZNG Value Message](#32-value-messages)
* The format of a UTF-8 string representing a [TZNG Value](#432-value-syntax) of that type

| Type       | ID |    N     |       ZNG Value Interpretation                 | TZNG Value Syntax                                             |
|------------|---:|:--------:|------------------------------------------------|---------------------------------------------------------------|
| `bool`     |  0 |    1     | one byte 0 (false) or 1 (true)                 | a single character `T` or `F`                                 |
| `byte`     |  1 |    1     | the byte                                       | two-characters of hexadecimal digit                           |
| `int16`    |  2 | variable | signed int of length N                         | decimal string representation of any signed, 16-bit integer   |
| `uint16`   |  3 | variable | unsigned int of length N                       | decimal string representation of any unsigned, 16-bit integer |
| `int32`    |  4 | variable | signed int of length N                         | decimal string representation of any signed, 32-bit integer   |
| `uint32`   |  5 | variable | unsigned int of length N                       | decimal string representation of any unsigned, 32-bit integer |
| `int64`    |  6 | variable | signed int of length N                         | decimal string representation of any signed, 64-bit integer   |
| `uint64`   |  7 | variable | unsigned int of length N                       | decimal string representation of any unsigned, 64-bit integer |
| `float64`  |  8 |    8     | 8 bytes of IEEE 64-bit format                  | decimal representation of a 64-bit IEEE floating point literal as defined in JavaScript |
| `string`   |  9 | variable | UTF-8 byte sequence of string                  | a UTF-8 string                                                |
| `bytes`    | 10 | variable | bytes of value                                 | a sequence of bytes encoded as base64                         |
| `bstring`  | 11 | variable | UTF-8 byte sequence with `\x` escapes          | a UTF-8 string with `\x` escapes of non-UTF binary data       |
| `enum `    | 12 | variable | UTF-8 bytes of enum string                     | a string representing an enumeration value defined outside the scope of ZNG |
| `ip`       | 13 | 4 or 16  | 4 or 16 bytes of IP address                    | a string representing an IP address in [IPv4 or IPv6 format](https://tools.ietf.org/html/draft-main-ipaddr-text-rep-02#section-3) |
| `port`     | 14 |    2     | unsigned int of length 2                       | decimal string representation of an unsigned, 16-bit integer  |
| `net`      | 15 | 8 or 32  | 8 or 32 bytes of IP prefix and subnet mask     | a string in CIDR notation representing an IP address and prefix length as defined in RFC 4632 and RFC 4291. |
| `time`     | 16 | variable | signed nanoseconds since epoch                 | signed dotted decimal notation of seconds                     |
| `duration` | 17 | variable | signed nanoseconds duration                    | signed dotted decimal notation of seconds                     |
| `null`     | 18 |    0     | No value, always represents an undefined value | must be the literal value `-`                                 |

## Appendix A. Related Links

* [Zeek ASCII logging](https://docs.zeek.org/en/stable/examples/logs/)
* [Binary logging in Zeek](https://old.zeek.org/development/projects/binary-logging.html)
* [Hadoop sequence file](https://cwiki.apache.org/confluence/display/HADOOP2/SequenceFile)
* [Avro](https://avro.apache.org)
* [Parquet](https://en.wikipedia.org/wiki/Apache_Parquet)
* [Protocol Buffers](https://developers.google.com/protocol-buffers)
* [MessagePack](https://msgpack.org/index.html)
* [gNMI](https://github.com/openconfig/reference/tree/master/rpc/gnmi)
