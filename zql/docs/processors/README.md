# Processors

A pipeline may contain one or more _processors_ to filter or transform event data. You can imagine the data flowing left-to-right through a processor, with its functionality further determined by arguments you may set. Processor names are case-insensitive.

The following available processors are documented in detail below:

* [`cut`](#cut)
* [`filter`](#filter)
* [`fuse`](#fuse)
* [`head`](#head)
* [`put`](#put)
* [`rename`](#rename)
* [`sort`](#sort)
* [`tail`](#tail)
* [`uniq`](#uniq)

**Note**: In the examples below, we'll use the `zq -f table` output format for human readability. Due to the width of the Zeek events used as sample data, you may need to "scroll right" in the output to see some field values.

**Note**: Per ZQL [search syntax](../search-syntax/README.md), many examples below use shorthand that leaves off the explicit leading `* |`, matching all events before invoking the first element in a pipeline.

---

# Available Processors

## `cut`

|                           |                                                             |
| ------------------------- | ----------------------------------------------------------- |
| **Description**           | Return the data only from the specified named fields.       |
| **Syntax**                | `cut <field-list>`                   |
| **Required<br>arguments** | `<field-list>`<br>One or more comma-separated field names or assignments.  |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/cut            |

#### Example #1:

To return only the `ts` and `uid` columns of `conn` events:

```zq-command
zq -f table 'cut ts,uid' conn.log.gz
```

#### Output:
```zq-output head:4
TS                          UID
2018-03-24T17:15:21.255387Z C8Tful1TvM3Zf5x8fl
2018-03-24T17:15:21.411148Z CXWfTK3LRdiuQxBbM6
2018-03-24T17:15:21.926018Z CM59GGQhNEoKONb5i
...
```

#### Example #2:

If the requested list of named fields returns no results, `cut` returns a warning.

```zq-command
zq -f table 'cut nothere,alsoabsent' weird.log.gz
```

#### Output:
```zq-output
cut: nothing found for nothere,alsoabsent
```

#### Example #3:

As long as some of the named fields are present, these will be returned. No warning is generated for absent fields in this case.

```zq-command
zq -f table 'cut nothere,name' weird.log.gz
```

#### Output:
```zq-output head:4
NAME
TCP_ack_underflow_or_misorder
truncated_header
above_hole_data_without_any_acks
...
```

#### Example #4:

To return only the `ts` and `uid` columns of `conn` events, with `ts` renamed to `time`:

```zq-command
zq -f table 'cut time=ts,uid' conn.log.gz
```

#### Output:
```zq-output head:4
TIME                        UID
2018-03-24T17:15:21.255387Z C8Tful1TvM3Zf5x8fl
2018-03-24T17:15:21.411148Z CXWfTK3LRdiuQxBbM6
2018-03-24T17:15:21.926018Z CM59GGQhNEoKONb5i
...
```

---

## `drop`

|                           |                                                             |
| ------------------------- | ----------------------------------------------------------- |
| **Description**           | Return the data only from all but the specified named fields.       |
| **Syntax**                | `drop <field-list>`                   |
| **Required<br>arguments** | `<field-list>`<br>One or more comma-separated field names or assignments.  |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc            |

#### Example #1:

To return all fields _other than_ the `_path` field and `id` record of `weird` events:

```zq-command
zq -f table 'drop _path,id' weird.log.gz
```

#### Output:
```zq-output head:4
TS                          UID                NAME                             ADDL             NOTICE PEER
2018-03-24T17:15:20.600843Z C1zOivgBT6dBmknqk  TCP_ack_underflow_or_misorder    -                F      zeek
2018-03-24T17:15:20.608108Z -                  truncated_header                 -                F      zeek
2018-03-24T17:15:20.610033Z C45Ff03lESjMQQQej1 above_hole_data_without_any_acks -                F      zeek
...
```

---

## `filter`

|                           |                                                                       |
| ------------------------- | --------------------------------------------------------------------- |
| **Description**           | Apply a search expression to potentially trim data from the pipeline. |
| **Syntax**                | `filter <search-expression>`                                          |
| **Required<br>arguments** | `<search-expression>`<br>Any valid expression in ZQL [search syntax](../search-syntax/README.md) |
| **Optional<br>arguments** | None                                                                  |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/filter                   |

#### Example #1:

To further trim the data returned in our [`cut`](#cut) example:

```zq-command
zq -f table 'cut ts,uid | filter uid=CXWfTK3LRdiuQxBbM6' conn.log.gz
```

#### Output:
```zq-output
TS                          UID
2018-03-24T17:15:21.411148Z CXWfTK3LRdiuQxBbM6
```

#### Example #2:

An alternative syntax for our [`and` operator example](../search-syntax/README.md#and):

```zq-command
zq -f table 'filter www.*cdn*.com _path=ssl' *.log.gz
```

#### Output:
```zq-output
_PATH TS                          UID                ID.ORIG_H   ID.ORIG_P ID.RESP_H    ID.RESP_P VERSION CIPHER                                CURVE     SERVER_NAME       RESUMED LAST_ALERT NEXT_PROTOCOL ESTABLISHED CERT_CHAIN_FUIDS                                                            CLIENT_CERT_CHAIN_FUIDS SUBJECT            ISSUER                                  CLIENT_SUBJECT CLIENT_ISSUER VALIDATION_STATUS
ssl   2018-03-24T17:23:00.244457Z CUG0fiQAzL4rNWxai  10.47.2.100 36150     52.85.83.228 443       TLSv12  TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 secp256r1 www.herokucdn.com F       -          h2            T           FXKmyTbr7HlvyL1h8,FADhCTvkq1ILFnD3j,FoVjYR16c3UIuXj4xk,FmiRYe1P53KOolQeVi   (empty)                 CN=*.herokucdn.com CN=Amazon,OU=Server CA 1B,O=Amazon,C=US -              -             ok
ssl   2018-03-24T17:24:00.189735Z CSbGJs3jOeB6glWLJj 10.47.7.154 27137     52.85.83.215 443       TLSv12  TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 secp256r1 www.herokucdn.com F       -          h2            T           FuW2cZ3leE606wXSia,Fu5kzi1BUwnF0bSCsd,FyTViI32zPvCmNXgSi,FwV6ff3JGj4NZcVPE4 (empty)                 CN=*.herokucdn.com CN=Amazon,OU=Server CA 1B,O=Amazon,C=US -              -             ok
```

---

## `fuse`

|                           |                                                   |
| ------------------------- | ------------------------------------------------- |
| **Description**           | Transforms input records into output records that unify the field and type information across all records in the query result. |
| **Syntax**                | `fuse`                                            |
| **Required<br>arguments** | None                                              |
| **Optional<br>arguments** | None                                              |
| **Limitations**           | Because `fuse` must make a first pass through the data to assemble the unified schema, results from queries that use `fuse` will not begin streaming back immediately. |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/fuse |

#### Example:

Let's say you'd started with table-formatted output of both `stats` and `weird` events:

```zq-command
zq -f table 'ts < 1521911721' stats.log.gz weird.log.gz
```

#### Output:
```zq-output
_PATH TS                          PEER MEM PKTS_PROC BYTES_RECV PKTS_DROPPED PKTS_LINK PKT_LAG EVENTS_PROC EVENTS_QUEUED ACTIVE_TCP_CONNS ACTIVE_UDP_CONNS ACTIVE_ICMP_CONNS TCP_CONNS UDP_CONNS ICMP_CONNS TIMERS ACTIVE_TIMERS FILES ACTIVE_FILES DNS_REQUESTS ACTIVE_DNS_REQUESTS REASSEM_TCP_SIZE REASSEM_FILE_SIZE REASSEM_FRAG_SIZE REASSEM_UNKNOWN_SIZE
stats 2018-03-24T17:15:20.600725Z zeek 74  26        29375      -            -         -       404         11            1                0                0                 1         0         0          36     32            0     0            0            0                   1528             0                 0                 0
_PATH TS                          UID                ID.ORIG_H   ID.ORIG_P ID.RESP_H      ID.RESP_P NAME                             ADDL NOTICE PEER
weird 2018-03-24T17:15:20.600843Z C1zOivgBT6dBmknqk  10.47.1.152 49562     23.217.103.245 80        TCP_ack_underflow_or_misorder    -    F      zeek
weird 2018-03-24T17:15:20.608108Z -                  -           -         -              -         truncated_header                 -    F      zeek
weird 2018-03-24T17:15:20.610033Z C45Ff03lESjMQQQej1 10.47.5.155 40712     91.189.91.23   80        above_hole_data_without_any_acks -    F      zeek
weird 2018-03-24T17:15:20.742818Z Cs7J9j2xFQcazrg7Nc 10.47.8.100 5900      10.129.53.65   58485     connection_originator_SYN_ack    -    F      zeek
```

Here a `stats` event was the first record type to be printed in the results stream, so the preceding header row describes the names of its fields. Then a `weird` event came next in the results stream, so a header row describing its fields was printed. This presentation accurately conveys the heterogeneous nature of the data, but changing schemas mid-stream is not allowed in formats such as CSV or other downstream tooling such as SQL. Indeed, `zq` halts its output in this case.

```
zq -f csv 'ts < 1521911721' stats.log.gz weird.log.gz
```

#### Output:
```
_path,ts,peer,mem,pkts_proc,bytes_recv,pkts_dropped,pkts_link,pkt_lag,events_proc,events_queued,active_tcp_conns,active_udp_conns,active_icmp_conns,tcp_conns,udp_conns,icmp_conns,timers,active_timers,files,active_files,dns_requests,active_dns_requests,reassem_tcp_size,reassem_file_size,reassem_frag_size,reassem_unknown_size,stats,2018-03-24T17:15:20.600725Z,zeek,74,26,29375,-,-,-,404,11,1,0,0,1,0,0,36,32,0,0,0,0,1528,0,0,0
csv output requires uniform records but different types encountered
```

By using `fuse`, the unified schema of fields and types across all records is assembled in a first pass through the data stream, which enables the presentation of the results under a single, wider header row with no further interruptions between the subsequent data rows.

```zq-command
zq -f csv 'ts < 1521911721 | fuse' stats.log.gz weird.log.gz
```

#### Output:
```zq-output
_path,ts,uid,id.orig_h,id.orig_p,id.resp_h,id.resp_p,name,addl,notice,peer,mem,pkts_proc,bytes_recv,pkts_dropped,pkts_link,pkt_lag,events_proc,events_queued,active_tcp_conns,active_udp_conns,active_icmp_conns,tcp_conns,udp_conns,icmp_conns,timers,active_timers,files,active_files,dns_requests,active_dns_requests,reassem_tcp_size,reassem_file_size,reassem_frag_size,reassem_unknown_size
stats,2018-03-24T17:15:20.600725Z,-,-,-,-,-,-,-,-,zeek,74,26,29375,-,-,-,404,11,1,0,0,1,0,0,36,32,0,0,0,0,1528,0,0,0
weird,2018-03-24T17:15:20.600843Z,C1zOivgBT6dBmknqk,10.47.1.152,49562,23.217.103.245,80,TCP_ack_underflow_or_misorder,-,F,zeek,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-
weird,2018-03-24T17:15:20.608108Z,-,-,-,-,-,truncated_header,-,F,zeek,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-
weird,2018-03-24T17:15:20.610033Z,C45Ff03lESjMQQQej1,10.47.5.155,40712,91.189.91.23,80,above_hole_data_without_any_acks,-,F,zeek,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-
weird,2018-03-24T17:15:20.742818Z,Cs7J9j2xFQcazrg7Nc,10.47.8.100,5900,10.129.53.65,58485,connection_originator_SYN_ack,-,F,zeek,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-,-
```

Other output formats invoked via `zq -f` that benefit greatly from the use of `fuse` include `table` and `zeek`.

---

## `head`

|                           |                                                                       |
| ------------------------- | --------------------------------------------------------------------- |
| **Description**           | Return only the first N events.                                       |
| **Syntax**                | `head [N]`                                                            |
| **Required<br>arguments** | None. If no arguments are specified, only the first event is returned.|
| **Optional<br>arguments** | `[N]`<br>An integer specifying the number of results to return. If not specified, defaults to `1`. |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/head                     |

#### Example #1:

To see the first `dns` event:

```zq-command
zq -f table 'head' dns.log.gz
```

#### Output:
```zq-output
_PATH TS                          UID                ID.ORIG_H   ID.ORIG_P ID.RESP_H  ID.RESP_P PROTO TRANS_ID RTT     QUERY          QCLASS QCLASS_NAME QTYPE QTYPE_NAME RCODE RCODE_NAME AA TC RD RA Z ANSWERS                        TTLS       REJECTED
dns   2018-03-24T17:15:20.865716Z C2zK5f13SbCtKcyiW5 10.47.1.100 41772     10.0.0.100 53        udp   36329    0.00087 ise.wrccdc.org 1      C_INTERNET  1     A          0     NOERROR    F  F  T  T  0 ise.wrccdc.cpp.edu,134.71.3.16 2230,41830 F
```

#### Example #2:

To see the first five `conn` events with activity on port `80`:

```zq-command
zq -f table 'id.resp_p=80 | head 5' conn.log.gz
```

#### Output:
```zq-output
_PATH TS                          UID                ID.ORIG_H     ID.ORIG_P ID.RESP_H   ID.RESP_P PROTO SERVICE DURATION ORIG_BYTES RESP_BYTES CONN_STATE LOCAL_ORIG LOCAL_RESP MISSED_BYTES HISTORY   ORIG_PKTS ORIG_IP_BYTES RESP_PKTS RESP_IP_BYTES TUNNEL_PARENTS
conn  2018-03-24T17:15:20.602122Z C4RZ6d4r5mJHlSYFI6 10.164.94.120 33299     10.47.3.200 80        tcp   -       0.003077 0          235        RSTO       -          -          0            ^dtfAR    4         208           4         678           -
conn  2018-03-24T17:15:20.606178Z CnKmhv4RfyAZ3fVc8b 10.164.94.120 36125     10.47.3.200 80        tcp   -       0.000002 0          0          RSTOS0     -          -          0            R         2         104           0         0             -
conn  2018-03-24T17:15:20.604325Z C65IMkEAWNlE1f6L8  10.164.94.120 45941     10.47.3.200 80        tcp   -       0.002708 0          242        RSTO       -          -          0            ^dtfAR    4         208           4         692           -
conn  2018-03-24T17:15:20.607031Z CpQfkTi8xytq87HW2  10.164.94.120 36729     10.47.3.200 80        tcp   http    0.006238 325        263        RSTO       -          -          0            ShADTdftR 10        1186          6         854           -
conn  2018-03-24T17:15:20.607695Z CpjMvj2Cvj048u6bF1 10.164.94.120 39169     10.47.3.200 80        tcp   http    0.007139 315        241        RSTO       -          -          0            ShADTdtfR 10        1166          6         810           -
```

---

## `put`

|                           |                                                 |
| ------------------------- | ----------------------------------------------- |
| **Description**           | Add/update fields based on the results of a computed expression |
| **Syntax**                | `put <field> = <expression> [, <field> = <expression> ...]`     |
| **Required arguments**    | `<field>`<br>Field into which the computed value will be stored.<br><br>`<expression>`<br>A valid ZQL [expression](../expressions/README.md). If evaluation of any expression fails, a warning is emitted and the original record is passed through unchanged. |
| **Optional arguments**    | None |
| **Limitations**           | If multiple fields are written in a single `put`, all the new field values are computed first and then they are all written simultaneously.  As a result, a computed value cannot be referenced in another expression.  If you need to re-use a computed result, this can be done by chaining multiple `put` processors.  For example, this will not work:<br>`put N=len(somelist), isbig=N>10`<br>But it could be written instead as:<br>`put N=len(somelist) \| put isbig=N>10` |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/put |

#### Example #1:

Compute a `total_bytes` field in `conn` records:

```zq-command
zq -q -f table 'put total_bytes = orig_bytes + resp_bytes | sort -r total_bytes | cut id, orig_bytes, resp_bytes, total_bytes' conn.log.gz
```

#### Output:
```zq-output head:5
ID.ORIG_H     ID.ORIG_P ID.RESP_H       ID.RESP_P ORIG_BYTES RESP_BYTES TOTAL_BYTES
10.47.7.154   27300     52.216.132.61   443       859        1781771107 1781771966
10.164.94.120 33691     10.47.3.200     80        355        1543916493 1543916848
10.47.8.100   37110     128.101.240.215 80        16398      376626606  376643004
10.47.3.151   11120     198.255.68.110  80        392        274063633  274064025
...
```

---

## `rename`

|                           |                                                 |
| ------------------------- | ----------------------------------------------- |
| **Description**           | Rename fields in a record. |
| **Syntax**                | `rename <newname> = <oldname> [, <newname> = <oldname> ...]`     |
| **Required arguments**    | One or more field assignment expressions. Renames are applied left to right; each rename observes the effect of all renames that preceded it. |
| **Optional arguments**    | None |
| **Limitations**           | A field can only be renamed within its own record. For example `id.orig_h` can be renamed to `id.src`, but it cannot be renamed to `src`. |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/rename |


#### Example:

Rename `ts` to `time`, rename one of the inner fields of `id`, and rename the `id` record itself to `conntuple`:

```zq-command
 zq -f table 'rename time=ts, id.src=id.orig_h, conntuple=id' conn.log.gz
```

#### Output:
```zq-output head:5
_PATH TIME                        UID                CONNTUPLE.SRC  CONNTUPLE.ORIG_P CONNTUPLE.RESP_H CONNTUPLE.RESP_P PROTO SERVICE  DURATION ORIG_BYTES RESP_BYTES CONN_STATE LOCAL_ORIG LOCAL_RESP MISSED_BYTES HISTORY     ORIG_PKTS ORIG_IP_BYTES RESP_PKTS RESP_IP_BYTES TUNNEL_PARENTS
conn  2018-03-24T17:15:21.255387Z C8Tful1TvM3Zf5x8fl 10.164.94.120  39681            10.47.3.155      3389             tcp   -        0.004266 97         19         RSTR       -          -          0            ShADTdtr    10        730           6         342           -
conn  2018-03-24T17:15:21.411148Z CXWfTK3LRdiuQxBbM6 10.47.25.80    50817            10.128.0.218     23189            tcp   -        0.000486 0          0          REJ        -          -          0            Sr          2         104           2         80            -
conn  2018-03-24T17:15:21.926018Z CM59GGQhNEoKONb5i  10.47.25.80    50817            10.128.0.218     23189            tcp   -        0.000538 0          0          REJ        -          -          0            Sr          2         104           2         80            -
conn  2018-03-24T17:15:22.690601Z CuKFds250kxFgkhh8f 10.47.25.80    50813            10.128.0.218     27765            tcp   -        0.000546 0          0          REJ        -          -          0            Sr          2         104           2         80            -
...
```

---

## `sort`

|                           |                                                                           |
| ------------------------- | ------------------------------------------------------------------------- |
| **Description**           | Sort events based on the order of values in the specified named field(s). |
| **Syntax**                | `sort [-r] [-nulls first\|last] [field-list]`                 |
| **Required<br>arguments** | None                                                                      |
| **Optional<br>arguments** | `[-r]`<br>If specified, results will be sorted in reverse order.<br><br>`[-nulls first\|last]`<br>Specifies where null values (i.e., values that are unset or that are not present at all in an incoming record) should be placed in the output.<br><br>`[field-list]`<br>One or more comma-separated field names by which to sort. Results will be sorted based on the values of the first field named in the list, then based on values in the second field named in the list, and so on.<br><br>If no field list is provided, sort will automatically pick a field by which to sort. The pick is done by examining the first result returned and finding the first field in left-to-right that is of one of the integer ZNG [data types](../data-types/README.md) (`int16`, `uint16`, `int32`, `uint32`, `int64`, `uint64`) and if no integer fields are found, the first `float64` field is used. If no fields of these numeric types are found, sorting will be performed on the first field found in left-to-right order that is _not_ of the `time` data type. |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/sort                         |

#### Example #1:

To sort `x509` events by `certificate.subject`:

```zq-command
zq -f table 'sort certificate.subject' x509.log.gz
```

#### Output:
```zq-output head:10
_PATH TS                          ID                 CERTIFICATE.VERSION CERTIFICATE.SERIAL                     CERTIFICATE.SUBJECT                                                                               CERTIFICATE.ISSUER                                                                                                                                       CERTIFICATE.NOT_VALID_BEFORE CERTIFICATE.NOT_VALID_AFTER CERTIFICATE.KEY_ALG CERTIFICATE.SIG_ALG     CERTIFICATE.KEY_TYPE CERTIFICATE.KEY_LENGTH CERTIFICATE.EXPONENT CERTIFICATE.CURVE SAN.DNS                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      SAN.URI SAN.EMAIL SAN.IP BASIC_CONSTRAINTS.CA BASIC_CONSTRAINTS.PATH_LEN
x509  2018-03-24T17:29:38.233315Z Fn2Gkp2Qd434JylJX9 3                   CB11D05B561B4BB1                       C=/C=US/ST=HI/O=Goldner and Sons/OU=1080p/CN=goldner.sons.net/emailAddress=1080p@goldner.sons.net C=/C=US/ST=HI/O=Goldner and Sons/OU=1080p/CN=goldner.sons.net/emailAddress=1080p@goldner.sons.net                                                        2016-05-09T10:09:02Z         2018-05-09T10:09:02Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 -                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            -       -         -      T                    -
x509  2018-03-24T17:18:48.524223Z Fxq7P31K2FS3v7CBSh 3                   031489479BCD9C116EA7B6162E5E68E6       CN=*.adnxs.com,O=AppNexus\\, Inc.,L=New York,ST=New York,C=US                                     CN=DigiCert ECC Secure Server CA,O=DigiCert Inc,C=US                                                                                                     2018-01-25T08:00:00Z         2019-01-25T20:00:00Z        id-ecPublicKey      ecdsa-with-SHA256       ecdsa                256                    -                    prime256v1        *.adnxs.com,adnxs.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        -       -         -      F                    -
x509  2018-03-24T17:18:48.524679Z F6WWPk3ajsHLrmNFdb 3                   031489479BCD9C116EA7B6162E5E68E6       CN=*.adnxs.com,O=AppNexus\\, Inc.,L=New York,ST=New York,C=US                                     CN=DigiCert ECC Secure Server CA,O=DigiCert Inc,C=US                                                                                                     2018-01-25T08:00:00Z         2019-01-25T20:00:00Z        id-ecPublicKey      ecdsa-with-SHA256       ecdsa                256                    -                    prime256v1        *.adnxs.com,adnxs.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        -       -         -      F                    -
x509  2018-03-24T17:29:40.661204Z FEMo0JLdFfaiP3cCj  3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:40.664443Z Fx9w2e3ZeGeRVzB7wa 3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:40.971149Z Fs71N02K3C48z0W8Rl 3                   08C2D95B922842FCD7EEC9C4AF3BB3C1       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:40.972007Z FNfnZ84jkUdb1ELG4e 3                   08C2D95B922842FCD7EEC9C4AF3BB3C1       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:41.350977Z FE774oxbdOCDlPx0i  3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:41.351155Z FQNOg4tbfGapYl4A7  3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
...
```

#### Example #2:

Now we'll sort `x509` events first by `certificate.subject`, then by the `id`. Compared to the previous example, note how this changes the order of some events that had the same `certificate.subject` value.

```zq-command
zq -f table 'sort certificate.subject,id' x509.log.gz
```

#### Output:
```zq-output head:10
_PATH TS                          ID                 CERTIFICATE.VERSION CERTIFICATE.SERIAL                     CERTIFICATE.SUBJECT                                                                               CERTIFICATE.ISSUER                                                                                                                                       CERTIFICATE.NOT_VALID_BEFORE CERTIFICATE.NOT_VALID_AFTER CERTIFICATE.KEY_ALG CERTIFICATE.SIG_ALG     CERTIFICATE.KEY_TYPE CERTIFICATE.KEY_LENGTH CERTIFICATE.EXPONENT CERTIFICATE.CURVE SAN.DNS                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      SAN.URI SAN.EMAIL SAN.IP BASIC_CONSTRAINTS.CA BASIC_CONSTRAINTS.PATH_LEN
x509  2018-03-24T17:29:38.233315Z Fn2Gkp2Qd434JylJX9 3                   CB11D05B561B4BB1                       C=/C=US/ST=HI/O=Goldner and Sons/OU=1080p/CN=goldner.sons.net/emailAddress=1080p@goldner.sons.net C=/C=US/ST=HI/O=Goldner and Sons/OU=1080p/CN=goldner.sons.net/emailAddress=1080p@goldner.sons.net                                                        2016-05-09T10:09:02Z         2018-05-09T10:09:02Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 -                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            -       -         -      T                    -
x509  2018-03-24T17:18:48.524679Z F6WWPk3ajsHLrmNFdb 3                   031489479BCD9C116EA7B6162E5E68E6       CN=*.adnxs.com,O=AppNexus\\, Inc.,L=New York,ST=New York,C=US                                     CN=DigiCert ECC Secure Server CA,O=DigiCert Inc,C=US                                                                                                     2018-01-25T08:00:00Z         2019-01-25T20:00:00Z        id-ecPublicKey      ecdsa-with-SHA256       ecdsa                256                    -                    prime256v1        *.adnxs.com,adnxs.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        -       -         -      F                    -
x509  2018-03-24T17:18:48.524223Z Fxq7P31K2FS3v7CBSh 3                   031489479BCD9C116EA7B6162E5E68E6       CN=*.adnxs.com,O=AppNexus\\, Inc.,L=New York,ST=New York,C=US                                     CN=DigiCert ECC Secure Server CA,O=DigiCert Inc,C=US                                                                                                     2018-01-25T08:00:00Z         2019-01-25T20:00:00Z        id-ecPublicKey      ecdsa-with-SHA256       ecdsa                256                    -                    prime256v1        *.adnxs.com,adnxs.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        -       -         -      F                    -
x509  2018-03-24T17:29:51.670293Z F0hybM3L5RvvQnB0Af 3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:51.670418Z F7QTmz23i9Wb9PxCec 3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:50.367386Z FAquaM1YmnRYGrPM0j 3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:41.350977Z FE774oxbdOCDlPx0i  3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:40.661204Z FEMo0JLdFfaiP3cCj  3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
x509  2018-03-24T17:29:51.317347Z FMITm2OyLT3OYnfq3  3                   068D4086AEB3472996E5DFA2EC521A41       CN=*.adobe.com,OU=IS,O=Adobe Systems Incorporated,L=San Jose,ST=California,C=US                   CN=DigiCert SHA2 Secure Server CA,O=DigiCert Inc,C=US                                                                                                    2018-01-05T08:00:00Z         2019-01-05T20:00:00Z        rsaEncryption       sha256WithRSAEncryption rsa                  2048                   65537                -                 *.adobe.com                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  -       -         -      F                    -
...
```

#### Example #3:

Here we'll find which originating IP addresses generated the most `conn` events using the `count()` [aggregate function](../aggregate-functions/README.md) and piping its output to a `sort` in reverse order. Note that even though we didn't list a field name as an explicit argument, the `sort` processor did what we wanted because it found a field of the `uint64` [data type](../data-types/README.md).

```zq-command
zq -f table 'count() by id.orig_h | sort -r' conn.log.gz
```

#### Output:
```zq-output head:5
ID.ORIG_H                COUNT
10.174.251.215           279014
10.47.24.81              162237
10.47.26.82              153056
10.224.110.133           67320
...
```

#### Example #4:

In this example we count the number of times each distinct username appears in `http` records, but deliberately put the unset username at the front of the list:

```zq-command
zq -f table 'count() by username | sort -nulls first username' http.log.gz
```

#### Output:
```zq-output
USERNAME     COUNT
-            139175
M32318       4854
agloop       1
cbucket      1
mteavee      1
poompaloompa 1
wwonka       1
```

---

## `tail`

|                           |                                                                       |
| ------------------------- | --------------------------------------------------------------------- |
| **Description**           | Return only the last N events.                                        |
| **Syntax**                | `tail [N]`                                                            |
| **Required<br>arguments** | None. If no arguments are specified, only the last event is returned. |
| **Optional<br>arguments** | `[N]`<br>An integer specifying the number of results to return. If not specified, defaults to `1`. |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/tail                     |

#### Example #1:

To see the last `dns` event:

```zq-command
zq -f table 'tail' dns.log.gz
```

#### Output:
```zq-output
_PATH TS                          UID                ID.ORIG_H    ID.ORIG_P ID.RESP_H ID.RESP_P PROTO TRANS_ID RTT QUERY           QCLASS QCLASS_NAME QTYPE QTYPE_NAME RCODE RCODE_NAME AA TC RD RA Z ANSWERS TTLS REJECTED
dns   2018-03-24T17:36:30.151237Z C0ybvu4HG3yWv6H5cb 172.31.255.5 60878     10.0.0.1  53        udp   36243    -   talk.google.com 1      C_INTERNET  1     A          -     -          F  F  T  F  0 -       -    F
```

#### Example #2:

To see the last five `conn` events with activity on port `80`:

```zq-command
zq -f table 'id.resp_p=80 | tail 5' conn.log.gz
```

#### Output:
```zq-output
_PATH TS                          UID                ID.ORIG_H      ID.ORIG_P ID.RESP_H    ID.RESP_P PROTO SERVICE DURATION  ORIG_BYTES RESP_BYTES CONN_STATE LOCAL_ORIG LOCAL_RESP MISSED_BYTES HISTORY    ORIG_PKTS ORIG_IP_BYTES RESP_PKTS RESP_IP_BYTES TUNNEL_PARENTS
conn  2018-03-24T17:33:23.087149Z CqPl942ft1MCpuNQgk 10.218.221.240 63812     10.47.2.20   80        tcp   -       15.607782 0          0          S1         -          -          0            Sh         2         88            10        440           -
conn  2018-03-24T17:36:25.557756Z CKCuBO2N2sY6m8qkv6 10.128.0.247   30549     10.47.22.65  80        tcp   http    0.006639  334        271        SF         -          -          0            ShADTftFa  10        1092          6         806           -
conn  2018-03-24T17:35:20.422826Z Cy1XB41BipfyCcCGVh 10.128.0.247   30487     10.47.2.58   80        tcp   http    68.309996 21249      15506      S1         -          -          0            ShADTadtTt 242       52202         270       41836         -
conn  2018-03-24T17:31:04.953409Z CMxwGp14TBAF3QtEq  10.219.216.224 56004     10.47.24.186 80        tcp   -       31.235313 0          0          S1         -          -          0            Sh         2         88            12        528           -
conn  2018-03-24T17:36:28.752765Z COICgc1FXHKteyFy67 10.0.0.227     61314     10.47.5.58   80        tcp   http    0.106754  1328       820        S1         -          -          0            ShADTadt   20        3720          12        2280          -
```

---

## `uniq`

|                           |                                                                       |
| ------------------------- | --------------------------------------------------------------------- |
| **Description**           | Remove adjacent duplicate events from the output, leaving only unique results.<br><br>Note that due to the large number of fields in typical events, and many fields whose values change often in subtle ways between events (e.g. timestamps), this processor will most often apply to the trimmed output from the [`cut`](#cut) processor. Furthermore, since duplicate field values may not often be adjacent to one another, upstream use of [`sort`](#sort) may also often be appropriate.
| **Syntax**                | `uniq [-c]`                                                           |
| **Required<br>arguments** | None                                                                  |
| **Optional<br>arguments** | `[-c]`<br>For each unique value shown, include a numeric count of how many times it appeared. |
| **Developer Docs**        | https://pkg.go.dev/github.com/brimsec/zq/proc/uniq                     |

#### Example:

To see a count of the top issuers of X.509 certificates:

```zq-command
zq -f table 'cut certificate.issuer | sort | uniq -c | sort -r' x509.log.gz
```

#### Output:
```zq-output head:3
CERTIFICATE.ISSUER                                                                                                                                       _UNIQ
O=VMware Installer                                                                                                                                       1761
CN=Snozberry                                                                                                                                             1108
...
```
