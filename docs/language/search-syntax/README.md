# Search Syntax

  * [Search all records](#search-all-records)
  * [Value Match](#value-match)
    + [Bare Word](#bare-word)
    + [Quoted Word](#quoted-word)
    + [Glob Wildcards](#glob-wildcards)
    + [Regular Expressions](#regular-expressions)
  * [Field/Value Match](#fieldvalue-match)
    + [Role of Data Types](#role-of-data-types)
    + [Finding Patterns with `matches`](#finding-patterns-with-matches)
    + [Containment](#containment)
    + [Comparisons](#comparisons)
    + [Other Examples](#other-examples)
  * [Boolean Logic](#boolean-logic)
    + [`and`](#and)
    + [`or`](#or)
    + [`not`](#not)
    + [Parentheses & Order of Evaluation](#parentheses--order-of-evaluation)

## Search all records

The simplest possible Zed search is a match of all records. This search is
expressed in `zq` with the wildcard `*`. The response will be a dump of all
records. The default `zq` output is binary [ZNG](../../formats/zng.md), a
compact format that's ideal for working in pipelines. However, in these docs
we'll often make use of the `-z` or `-Z` options to output the text-based
[ZSON](../../formats/zson.md) format, which is readable at the command line.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z '*' schools.zson
```

#### Output:
```mdtest-output head
{School:"'3R' Middle",District:"Nevada County Office of Education",City:"Nevada City",County:"Nevada",Zip:"95959",Latitude:null (float64),Longitude:null (float64),Magnet:null (bool),OpenDate:1995-10-30T00:00:00Z,ClosedDate:1996-06-28T00:00:00Z,Phone:null (string),StatusType:"Merged",Website:null (string)} (=school)
{School:"100 Black Men of the Bay Area Community",District:"Oakland Unified",City:"Oakland",County:"Alameda",Zip:"94607-1404",Latitude:37.745418,Longitude:-122.14067,Magnet:null,OpenDate:2012-08-06T00:00:00Z,ClosedDate:2014-10-28T00:00:00Z,Phone:null,StatusType:"Closed",Website:"www.100school.org"} (school)
{School:"101 Elementary",District:"Victor Elementary",City:"Victorville",County:"San Bernardino",Zip:"92395-3360",Latitude:null,Longitude:null,Magnet:null,OpenDate:1996-02-07T00:00:00Z,ClosedDate:2005-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:"www.charter101.org"} (school)
{School:"180 Program",District:"Novato Unified",City:"Novato",County:"Marin",Zip:"94947-4004",Latitude:38.097792,Longitude:-122.57617,Magnet:null,OpenDate:2012-08-22T00:00:00Z,ClosedDate:2014-06-13T00:00:00Z,Phone:null,StatusType:"Closed",Website:null} (school)
...
```

If the query argument is left out entirely, this wildcard is the default
search. The following shorthand command line would produce the same output
shown above.

```
zq -z schools.zson
```

To start a Zed pipeline with this default search, you can similarly leave out
the leading `* |` before invoking your first
[operator](#../operators/README.md) or
[aggregate function](#../aggregate-functions/README.md). The following example
is shorthand for:

```
zq -z '* | cut School,City' schools.zson
```

#### Example:

```mdtest-command zed-sample-data/edu/zson
zq -z 'cut School,City' schools.zson
```

#### Output:
```mdtest-output head
{School:"'3R' Middle",City:"Nevada City"}
{School:"100 Black Men of the Bay Area Community",City:"Oakland"}
{School:"101 Elementary",City:"Victorville"}
...
```

## Value Match

The search result can be narrowed to include only records that contain certain
values in any field(s).

### Bare Word

The simplest form of such a search is a _bare_ word (not wrapped in quotes),
which will match against any field that contains the word, whether it's an
exact match to the data type and value of a field or the word appears as a
substring in a field.

For example, searching across all our logs for `596` matches records that
contain numeric fields of this precise value (such as from the SAT test scores
in our sample data) and also where it appears within string-typed fields (such
as the zip code and phone number fields.)

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z '596' *.zson
```

#### Output:
```mdtest-output head
{AvgScrMath:591 (uint16),AvgScrRead:610 (uint16),AvgScrWrite:596 (uint16),cname:"Los Angeles",dname:"William S. Hart Union High",sname:"Academy of the Canyons"} (=satscore)
{AvgScrMath:614,AvgScrRead:596,AvgScrWrite:592,cname:"Alameda",dname:"Pleasanton Unified",sname:"Amador Valley High"} (satscore)
{AvgScrMath:620,AvgScrRead:596,AvgScrWrite:590,cname:"Yolo",dname:"Davis Joint Unified",sname:"Davis Senior High"} (satscore)
{School:"Achieve Charter School of Paradise Inc.",District:"Paradise Unified",City:"Paradise",County:"Butte",Zip:"95969-3913",Latitude:39.760323,Longitude:-121.62078,Magnet:false,OpenDate:2005-09-12T00:00:00Z,ClosedDate:null (time),Phone:"(530) 872-4100",StatusType:"Active",Website:"www.achievecharter.org"} (=school)
{School:"Alliance Ouchi-O'Donovan 6-12 Complex",District:"Los Angeles Unified",City:"Los Angeles",County:"Los Angeles",Zip:"90043-2622",Latitude:33.993484,Longitude:-118.32246,Magnet:false,OpenDate:2006-09-05T00:00:00Z,ClosedDate:null,Phone:"(323) 596-2290",StatusType:"Active",Website:"http://ouchihs.org"} (school)
...
```

By comparison, the section below on [Field/Value Match](#fieldvalue-match)
describes ways to perform searches against only fields of a specific
[data type](../data-types/README.md).

### Quoted Word

Sometimes you may need to search for sequences of multiple words or words that
contain special characters. To achieve this, wrap your search term in quotes.

Let's say we've noticed that a couple of the school names in our sample data
include the string `Defunct=`. An attempt to enter this as a [bare word](#bare-word)
search causes an error because the language parser interprets this as the
start of an attempted [field/value match](#fieldvalue-match) for a field named
`Defunct`.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'Defunct=' *.zson || true
```

#### Output:
```mdtest-output
zq: error parsing Zed at column 8:
Defunct=
   === ^ ===
```

However, wrapping in quotes gives the desired result.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z '"Defunct="' schools.zson
```

#### Output:
```mdtest-output
{School:"Lincoln Elem 'Defunct=",District:"Modesto City Elementary",City:null (string),County:"Stanislaus",Zip:null (string),Latitude:null (float64),Longitude:null (float64),Magnet:null (bool),OpenDate:1980-07-01T00:00:00Z,ClosedDate:1989-06-30T00:00:00Z,Phone:null (string),StatusType:"Closed",Website:null (string)} (=school)
{School:"Lovell Elem 'Defunct=",District:"Cutler-Orosi Joint Unified",City:null,County:"Tulare",Zip:null,Latitude:null,Longitude:null,Magnet:null,OpenDate:1980-07-01T00:00:00Z,ClosedDate:1989-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:null} (school)
```

Wrapping in quotes is particularly handy when you're looking for long, specific
strings that may have several special characters in them. For example, let's
say we're looking for information on the Union Hill Elementary district.
Entered without quotes, we end up matching way more records than we intended
since each space character between words is treated as a [boolean `and`](#and).

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'Union Hill Elementary' schools.zson
```

#### Output:
```mdtest-output head
{School:"A. M. Thomas Middle",District:"Lost Hills Union Elementary",City:"Lost Hills",County:"Kern",Zip:"93249-0158",Latitude:35.615269,Longitude:-119.69955,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null (time),Phone:"(661) 797-2626",StatusType:"Active",Website:null (string)} (=school)
{School:"Alview Elementary",District:"Alview-Dairyland Union Elementary",City:"Chowchilla",County:"Madera",Zip:"93610-9225",Latitude:37.050632,Longitude:-120.4734,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(559) 665-2275",StatusType:"Active",Website:null} (school)
{School:"Anaverde Hills",District:"Westside Union Elementary",City:"Palmdale",County:"Los Angeles",Zip:"93551-5518",Latitude:34.564651,Longitude:-118.18012,Magnet:false,OpenDate:2005-08-15T00:00:00Z,ClosedDate:null,Phone:"(661) 575-9923",StatusType:"Active",Website:null} (school)
{School:"Apple Blossom",District:"Twin Hills Union Elementary",City:"Sebastopol",County:"Sonoma",Zip:"95472-3917",Latitude:38.387396,Longitude:-122.84954,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(707) 823-1041",StatusType:"Active",Website:null} (school)
...
```

However, wrapping the entire term in quotes allows us to search for the
complete string, including the spaces.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z '"Union Hill Elementary"' schools.zson
```

#### Output:
```mdtest-output
{School:"Highland Oaks Elementary",District:"Union Hill Elementary",City:"Grass Valley",County:"Nevada",Zip:"95945",Latitude:null (float64),Longitude:null (float64),Magnet:null (bool),OpenDate:1997-09-02T00:00:00Z,ClosedDate:2003-07-02T00:00:00Z,Phone:null (string),StatusType:"Closed",Website:null (string)} (=school)
{School:"Union Hill 3R Community Day",District:"Union Hill Elementary",City:"Grass Valley",County:"Nevada",Zip:"95945",Latitude:39.229055,Longitude:-121.07127,Magnet:null,OpenDate:2003-08-20T00:00:00Z,ClosedDate:2011-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:"www.uhsd.k12.ca.us"} (school)
{School:"Union Hill Charter Home",District:"Union Hill Elementary",City:"Grass Valley",County:"Nevada",Zip:"95945-8805",Latitude:39.204457,Longitude:-121.03829,Magnet:false,OpenDate:1995-07-14T00:00:00Z,ClosedDate:2015-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:"www.uhsd.k12.ca.us"} (school)
{School:"Union Hill Elementary",District:"Union Hill Elementary",City:"Grass Valley",County:"Nevada",Zip:"95945-8805",Latitude:39.204457,Longitude:-121.03829,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(530) 273-8456",StatusType:"Active",Website:"www.uhsd.k12.ca.us"} (school)
{School:"Union Hill Middle",District:"Union Hill Elementary",City:"Grass Valley",County:"Nevada",Zip:"94945-8805",Latitude:39.205006,Longitude:-121.03778,Magnet:false,OpenDate:2013-08-14T00:00:00Z,ClosedDate:null,Phone:"(530) 273-8456",StatusType:"Active",Website:"www.uhsd.k12.ca.us"} (school)
{School:null,District:"Union Hill Elementary",City:"Grass Valley",County:"Nevada",Zip:"95945-8730",Latitude:39.208869,Longitude:-121.03551,Magnet:null,OpenDate:null,ClosedDate:null,Phone:"(530) 273-0647",StatusType:"Active",Website:"www.uhsd.k12.ca.us"} (school)
```

### Glob Wildcards

To find values that may contain arbitrary substrings between or alongside the
desired word(s), one or more
[glob](https://en.wikipedia.org/wiki/Glob_(programming))-style wildcards can be
used.

For example, the following search finds records that contain school names
that have some additional text between `ACE` and `Academy`.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'ACE*Academy' schools.zson
```

#### Output:
```mdtest-output head
{School:"ACE Empower Academy",District:"Santa Clara County Office of Education",City:"San Jose",County:"Santa Clara",Zip:"95116-3423",Latitude:37.348601,Longitude:-121.8446,Magnet:false,OpenDate:2008-08-26T00:00:00Z,ClosedDate:null (time),Phone:"(408) 729-3920",StatusType:"Active",Website:"www.acecharter.org"} (=school)
{School:"ACE Inspire Academy",District:"San Jose Unified",City:"San Jose",County:"Santa Clara",Zip:"95112-6334",Latitude:37.350981,Longitude:-121.87205,Magnet:false,OpenDate:2015-08-03T00:00:00Z,ClosedDate:null,Phone:"(408) 295-6008",StatusType:"Active",Website:"www.acecharter.org"} (school)
```

> **Note:** Our use of `*` to [search all records](#search-all-records) as
> shown previously is the simplest example of using a glob wildcard.

Glob wildcards only have effect when used with [bare word](#bare-word)
searches. An asterisk in a [quoted word](#quoted-word) search will match
explicitly against an asterisk character.

### Regular Expressions

For matching that requires more precision than can be achieved with
[glob wildcards](#glob-wildcards), regular expressions (regexps) are also
available. To use them, simply place a `/` character before and after the
regexp.

For example, since there's so many high schools in our sample data, to find
only records containing strings that _begin_ with the word `High`:

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z '/^High /' schools.zson
```

#### Output:
```mdtest-output head
{School:"High Desert",District:"Soledad-Agua Dulce Union Eleme",City:"Acton",County:"Los Angeles",Zip:"93510",Latitude:34.490977,Longitude:-118.19646,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:1993-06-30T00:00:00Z,Phone:null (string),StatusType:"Merged",Website:null (string)} (=school)
{School:"High Desert",District:"Acton-Agua Dulce Unified",City:"Acton",County:"Los Angeles",Zip:"93510-1757",Latitude:34.492578,Longitude:-118.19039,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(661) 269-0310",StatusType:"Active",Website:null} (school)
{School:"High Desert Academy",District:"Eastern Sierra Unified",City:"Benton",County:"Mono",Zip:"93512-0956",Latitude:37.818597,Longitude:-118.47712,Magnet:null,OpenDate:1996-09-03T00:00:00Z,ClosedDate:2012-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:"www.esusd.org"} (school)
{School:"High Desert Academy of Applied Arts and Sciences",District:"Victor Valley Union High",City:"Victorville",County:"San Bernardino",Zip:"92394",Latitude:34.531144,Longitude:-117.31697,Magnet:null,OpenDate:2004-09-07T00:00:00Z,ClosedDate:2011-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:"www.hdaaas.org"} (school)
...
```

Regexps are a detailed topic all their own. For details, reference the
[documentation for re2](https://github.com/google/re2/wiki/Syntax), which is
the library that Zed uses to provide regexp support.

## Field/Value Match

The search result can be narrowed to include only records that contain a
certain value in a particular named field. For example, the following search
will only match records containing the field called `District` where it is set
to the precise string value `Winton`.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'District=="Winton"' schools.zson
```

#### Output:

```mdtest-output
{School:"Frank Sparkes Elementary",District:"Winton",City:"Winton",County:"Merced",Zip:"95388-0008",Latitude:37.382084,Longitude:-120.61847,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null (time),Phone:"(209) 357-6180",StatusType:"Active",Website:null (string)} (=school)
{School:"Sybil N. Crookham Elementary",District:"Winton",City:"Winton",County:"Merced",Zip:"95388-0130",Latitude:37.389501,Longitude:-120.61636,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(209) 357-6182",StatusType:"Active",Website:null} (school)
{School:"Winfield Elementary",District:"Winton",City:"Winton",County:"Merced",Zip:"95388",Latitude:37.389121,Longitude:-120.60442,Magnet:false,OpenDate:2007-08-13T00:00:00Z,ClosedDate:null,Phone:"(209) 357-6891",StatusType:"Active",Website:null} (school)
{School:"Winton Middle",District:"Winton",City:"Winton",County:"Merced",Zip:"95388-1477",Latitude:37.379938,Longitude:-120.62263,Magnet:false,OpenDate:1990-07-20T00:00:00Z,ClosedDate:null,Phone:"(209) 357-6189",StatusType:"Active",Website:null} (school)
{School:null,District:"Winton",City:"Winton",County:"Merced",Zip:"95388-0008",Latitude:37.389467,Longitude:-120.6147,Magnet:null,OpenDate:null,ClosedDate:null,Phone:"(209) 357-6175",StatusType:"Active",Website:"www.winton.k12.ca.us"} (school)
```

Because the right-hand-side value to which we were comparing was a string, it
was necessary to wrap it in quotes. If we'd left it bare, it would have been
interpreted as a field name.

For example, to see the records in which the school and district name are the
same:

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'School==District' schools.zson
```

#### Output:

```mdtest-output head
{School:"Adelanto Elementary",District:"Adelanto Elementary",City:"Adelanto",County:"San Bernardino",Zip:"92301-1734",Latitude:34.576166,Longitude:-117.40944,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null (time),Phone:"(760) 246-5892",StatusType:"Active",Website:null (string)} (=school)
{School:"Allensworth Elementary",District:"Allensworth Elementary",City:"Allensworth",County:"Tulare",Zip:"93219-9709",Latitude:35.864487,Longitude:-119.39068,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(661) 849-2401",StatusType:"Active",Website:null} (school)
{School:"Alta Loma Elementary",District:"Alta Loma Elementary",City:"Alta Loma",County:"San Bernardino",Zip:"91701-5007",Latitude:34.12597,Longitude:-117.59744,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(909) 484-5000",StatusType:"Active",Website:null} (school)
...
```

### Role of Data Types

To match successfully when comparing values to the contents of named fields,
the value must be comparable to the _data type_ of the field.

For instance, the 'Zip' field in our schools data is of `string` type because
several values are of the extended format that includes a hyphen and four
additional digits and hence could not be represented in a numeric type.

```mdtest-command zed-sample-data/edu/zson
zq -z 'cut Zip' schools.zson
```

#### Output:
```mdtest-output head
{Zip:"95959"}
{Zip:"94607-1404"}
{Zip:"92395-3360"}
...
```

An attempted [field/value match](#fieldvalue-match) `Zip==95959` would _not_
match the top record shown, since Zed recognizes the bare value `95959` as a
number before comparing it to all the fields named `Zip` that it sees in the
input stream. However, `Zip=="95959"` _would_ match, since the quotes cause Zed
to treat the value as a string.

See the [Data Types](../data-types/README.md) page for more details.

### Finding Patterns with `matches`

When comparing a named field to a quoted value, the quoted value is treated as
an _exact_ match.

For example, let's say we know there's several schools that start with
`Luther`, but only a couple districts do. Because `Luther` only appears as a
_substring_ of the district names in our sample data, the following example
produces no output.

#### Example:

```mdtest-command zed-sample-data/edu/zson
zq -z 'District=="Luther"' schools.zson
```

#### Output:
```mdtest-output
```

To achieve this with a field/value match, we enter `matches` before specifying
a [glob wildcard](#glob-wildcards).

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'District matches Luther*' schools.zson
```

#### Output:

```mdtest-output head
{School:"Luther Burbank Elementary",District:"Luther Burbank",City:"San Jose",County:"Santa Clara",Zip:"95128-1931",Latitude:37.323556,Longitude:-121.9267,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null (time),Phone:"(408) 295-1814",StatusType:"Active",Website:null (string)} (=school)
{School:null,District:"Luther Burbank",City:"San Jose",County:"Santa Clara",Zip:"95128-1931",Latitude:37.323556,Longitude:-121.9267,Magnet:null,OpenDate:null,ClosedDate:null,Phone:"(408) 295-2450",StatusType:"Active",Website:"www.lbsd.k12.ca.us"} (school)
```

[Regular expressions](#regular-expressions) can also be used with `matches`.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'School matches /^Sunset (Ranch|Ridge) Elementary/' schools.zson
```

#### Output:
```mdtest-output
{School:"Sunset Ranch Elementary",District:"Rocklin Unified",City:"Rocklin",County:"Placer",Zip:"95765-5441",Latitude:38.826425,Longitude:-121.2864,Magnet:false,OpenDate:2010-08-17T00:00:00Z,ClosedDate:null (time),Phone:"(916) 624-2048",StatusType:"Active",Website:"www.rocklin.k12.ca.us"} (=school)
{School:"Sunset Ridge Elementary",District:"Pacifica",City:"Pacifica",County:"San Mateo",Zip:"94044-2029",Latitude:37.653836,Longitude:-122.47919,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(650) 738-6687",StatusType:"Active",Website:null} (school)
```

### Containment

Rather than testing for strict equality or pattern matches, you may want to
determine if a value is among the many possible elements of a complex field.
This is performed with `in`.

Since our sample data doesn't contain complex fields, we'll make one by
using the [`union`](../aggregate-functions/#union) aggregate function to
create a [`set`](https://github.com/brimdata/zed/blob/main/docs/formats/zson.md#343-set-value)-typed
field called `Schools` that contains all unique school names per district. From
these we'll find each set that contains a school named `Lincoln Elementary`.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -Z 'Schools:=union(School) by District | sort | "Lincoln Elementary" in Schools' schools.zson
```

#### Output:
```mdtest-output head
{
    District: "Alpine County Unified",
    Schools: |[
        "",
        "Woodfords High",
        "Clay Elementary",
        "Bear Valley High",
        "Lincoln Elementary",
        "Jmms Satellite Campus",
        "Bear Valley Elementary",
        "Diamond Valley Elementary",
        "Kirkwood Meadows Elementary",
        "Alpine County Special Education",
        "Diamond Valley Independent Study",
        "Alpine County Secondary Community Day",
        "Alpine County Elementary Community Day"
    ]|
}
...
```

Determining whether the value of an `ip`-type field is contained within a
subnet also uses `in`.

The following example locates all schools whose web sites are hosted in an
IP address in the class A `38`.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'addr in 38.0.0.0/8' webaddrs.zson
```

#### Output:
```mdtest-output
{Website:"www.learningchoice.org",addr:38.95.129.245}
{Website:"www.mpcsd.org",addr:38.102.147.181}
```

### Comparisons

In addition to testing for equality via `==` and finding patterns via
`matches`, the other common methods of comparison `!=`, `<`, `>`, `<=`, and
`>=` are also available.

For example, the following search finds the schools that reported the highest
SAT math scores.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'AvgScrMath > 690' satscores.zson
```

#### Output:
```mdtest-output
{AvgScrMath:698 (uint16),AvgScrRead:639 (uint16),AvgScrWrite:664 (uint16),cname:"Santa Clara",dname:"Fremont Union High",sname:"Lynbrook High"} (=satscore)
{AvgScrMath:699,AvgScrRead:653,AvgScrWrite:671,cname:"Alameda",dname:"Fremont Unified",sname:"Mission San Jose High"} (satscore)
{AvgScrMath:691,AvgScrRead:638,AvgScrWrite:657,cname:"Santa Clara",dname:"Fremont Union High",sname:"Monta Vista High"} (satscore)
```

The same approach can be used to compare characters in `string`-type values,
such as this search that finds school names at the high end of the alphabet.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'School > "Z"' schools.zson
```

#### Output:
```mdtest-output head
{School:"Zamora Elementary",District:"Woodland Joint Unified",City:"Woodland",County:"Yolo",Zip:"95695-5137",Latitude:38.658609,Longitude:-121.79355,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null (time),Phone:"(530) 666-3641",StatusType:"Active",Website:null (string)} (=school)
{School:"Zamorano Elementary",District:"San Diego Unified",City:"San Diego",County:"San Diego",Zip:"92139-2989",Latitude:32.680338,Longitude:-117.03864,Magnet:true,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(619) 430-1400",StatusType:"Active",Website:"http://new.sandi.net/schools/zamorano"} (school)
{School:"Zane (Catherine L.) Junior High",District:"Eureka City High",City:"Eureka",County:"Humboldt",Zip:"95501-3140",Latitude:40.788118,Longitude:-124.14903,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:1998-06-30T00:00:00Z,Phone:null,StatusType:"Merged",Website:null} (school)
...
```

## Boolean Logic

Your searches can be further refined by using boolean keywords `and`, `or`,
and `not`. These are case-insensitive, so `AND`, `OR`, and `NOT` can also be
used.

### `and`

If you enter multiple [value match](#value-match) or
[field/value match](#fieldvalue-match) terms separated by blank space, Zed
implicitly applies a boolean `and` between them, such that records are only
returned if they match on _all_ terms.

For example, let's say we're searching for info about academies that are
flagged as being in a `Pending` status.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'StatusType=="Pending" academy' schools.zson
```

#### Output:
```mdtest-output
{School:"Equitas Academy 4",District:"Los Angeles Unified",City:"Los Angeles",County:"Los Angeles",Zip:"90015-2412",Latitude:34.044837,Longitude:-118.27844,Magnet:false,OpenDate:2017-09-01T00:00:00Z,ClosedDate:null (time),Phone:"(213) 201-0440",StatusType:"Pending",Website:"http://equitasacademy.org"} (=school)
{School:"Pinnacle Academy Charter - Independent Study",District:"South Monterey County Joint Union High",City:"King City",County:"Monterey",Zip:"93930-3311",Latitude:36.208934,Longitude:-121.13286,Magnet:false,OpenDate:2016-08-08T00:00:00Z,ClosedDate:null,Phone:"(831) 385-4661",StatusType:"Pending",Website:"www.smcjuhsd.org"} (school)
{School:"Rocketship Futuro Academy",District:"SBE - Rocketship Futuro Academy",City:"Concord",County:"Contra Costa",Zip:"94521-1522",Latitude:37.965658,Longitude:-121.96106,Magnet:false,OpenDate:2016-08-15T00:00:00Z,ClosedDate:null,Phone:"(301) 789-5469",StatusType:"Pending",Website:"www.rsed.org"} (school)
{School:"Sherman Thomas STEM Academy",District:"Madera Unified",City:"Madera",County:"Madera",Zip:"93638",Latitude:36.982843,Longitude:-120.06665,Magnet:false,OpenDate:2017-08-09T00:00:00Z,ClosedDate:null,Phone:"(559) 674-1192",StatusType:"Pending",Website:"www.stcs.k12.ca.us"} (school)
{School:null,District:"SBE - Rocketship Futuro Academy",City:"Concord",County:"Contra Costa",Zip:"94521-1522",Latitude:37.965658,Longitude:-121.96106,Magnet:null,OpenDate:null,ClosedDate:null,Phone:"(301) 789-5469",StatusType:"Pending",Website:"www.rsed.org"} (school)
```

> **Note:** You may also include `and` explicitly if you wish:
> ```
> StatusType=="Pending" and academy
> ```

### `or`

`or` returns the union of the matches from multiple terms.

For example, we can revisit two of our previous example searches that each only
returned a couple records, searching now with `or` to see them all at once.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z '"Defunct=" or ACE*Academy' schools.zson
```

#### Output:

```mdtest-output
{School:"ACE Empower Academy",District:"Santa Clara County Office of Education",City:"San Jose",County:"Santa Clara",Zip:"95116-3423",Latitude:37.348601,Longitude:-121.8446,Magnet:false,OpenDate:2008-08-26T00:00:00Z,ClosedDate:null (time),Phone:"(408) 729-3920",StatusType:"Active",Website:"www.acecharter.org"} (=school)
{School:"ACE Inspire Academy",District:"San Jose Unified",City:"San Jose",County:"Santa Clara",Zip:"95112-6334",Latitude:37.350981,Longitude:-121.87205,Magnet:false,OpenDate:2015-08-03T00:00:00Z,ClosedDate:null,Phone:"(408) 295-6008",StatusType:"Active",Website:"www.acecharter.org"} (school)
{School:"Lincoln Elem 'Defunct=",District:"Modesto City Elementary",City:null,County:"Stanislaus",Zip:null,Latitude:null,Longitude:null,Magnet:null,OpenDate:1980-07-01T00:00:00Z,ClosedDate:1989-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:null} (school)
{School:"Lovell Elem 'Defunct=",District:"Cutler-Orosi Joint Unified",City:null,County:"Tulare",Zip:null,Latitude:null,Longitude:null,Magnet:null,OpenDate:1980-07-01T00:00:00Z,ClosedDate:1989-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:null} (school)
```

### `not`

Use `not` to invert the matching logic in the term that comes to the right of
it in your search.

For example, to find schools in the `Dixon Unified` district _other than_
elementary schools, we invert the logic of a search term.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'not elementary District=="Dixon Unified"' schools.zson
```

#### Output:

```mdtest-output head
{School:"C. A. Jacobs Intermediate",District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620-3209",Latitude:38.446472,Longitude:-121.83631,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null (time),Phone:"(707) 693-6350",StatusType:"Active",Website:"www.dixonusd.org"} (=school)
{School:"Dixon Adult",District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620",Latitude:38.444818,Longitude:-121.82287,Magnet:null,OpenDate:1996-09-09T00:00:00Z,ClosedDate:2016-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:null} (school)
{School:"Dixon Community Day",District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620",Latitude:38.44755,Longitude:-121.82001,Magnet:false,OpenDate:2003-08-23T00:00:00Z,ClosedDate:null,Phone:"(707) 693-6340",StatusType:"Active",Website:"www.dixonusd.org"} (school)
{School:"Dixon High",District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620-9301",Latitude:38.436088,Longitude:-121.81672,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(707) 693-6330",StatusType:"Active",Website:null} (school)
{School:"Dixon Montessori Charter",District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620-2702",Latitude:38.447984,Longitude:-121.83186,Magnet:false,OpenDate:2010-08-11T00:00:00Z,ClosedDate:null,Phone:"(707) 678-8953",StatusType:"Active",Website:"www.dixonmontessori.org"} (school)
{School:"Dixon Unified Alter. Educ.",District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620",Latitude:null,Longitude:null,Magnet:null,OpenDate:1993-08-26T00:00:00Z,ClosedDate:1994-06-30T00:00:00Z,Phone:null,StatusType:"Closed",Website:null} (school)
{School:"Maine Prairie High (Continuation)",District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620-3019",Latitude:38.447549,Longitude:-121.81986,Magnet:false,OpenDate:1980-07-01T00:00:00Z,ClosedDate:null,Phone:"(707) 693-6340",StatusType:"Active",Website:null} (school)
{School:null,District:"Dixon Unified",City:"Dixon",County:"Solano",Zip:"95620-3447",Latitude:38.44468,Longitude:-121.82249,Magnet:null,OpenDate:null,ClosedDate:null,Phone:"(707) 693-6300",StatusType:"Active",Website:"www.dixonusd.org"} (school)
```

> **Note:** `!` can also be used as alternative shorthand for `not`.
> ```
> ! elementary District=="Dixon Unified"
> ```

### Parentheses & Order of Evaluation

Unless wrapped in parentheses, a search is evaluated in _left-to-right order_.
Terms wrapped in parentheses will be evaluated _first_, overriding the default
left-to-right evaluation.

For example, we've noticed there are some test score records that have `null`
values for all three test scores.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'AvgScrMath==null AvgScrRead==null AvgScrWrite==null' satscores.zson
```

#### Output:
```mdtest-output head
{AvgScrMath:null (uint16),AvgScrRead:null (uint16),AvgScrWrite:null (uint16),cname:"Riverside",dname:"Beaumont Unified",sname:"21st Century Learning Institute"} (=satscore)
{AvgScrMath:null,AvgScrRead:null,AvgScrWrite:null,cname:"Los Angeles",dname:"ABC Unified",sname:"ABC Secondary (Alternative)"} (satscore)
...
```

We can easily filter these out by negating the search for these records.


#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z 'not (AvgScrMath==null AvgScrRead==null AvgScrWrite==null)' satscores.zson
```

#### Output:
```mdtest-output head
{AvgScrMath:371 (uint16),AvgScrRead:376 (uint16),AvgScrWrite:368 (uint16),cname:"Los Angeles",dname:"Los Angeles Unified",sname:"APEX Academy"} (=satscore)
{AvgScrMath:367,AvgScrRead:359,AvgScrWrite:369,cname:"Alameda",dname:"Oakland Unified",sname:"ARISE High"} (satscore)
{AvgScrMath:491,AvgScrRead:489,AvgScrWrite:484,cname:"Santa Clara",dname:"San Jose Unified",sname:"Abraham Lincoln High"} (satscore)
...
```

Parentheses can also be nested.

#### Example:
```mdtest-command zed-sample-data/edu/zson
zq -z '(sname matches *High*) and (not (AvgScrMath==null AvgScrRead==null AvgScrWrite==null) and dname=="San Francisco Unified")' satscores.zson
```

#### Output:
```mdtest-output head
{AvgScrMath:504 (uint16),AvgScrRead:467 (uint16),AvgScrWrite:467 (uint16),cname:"San Francisco",dname:"San Francisco Unified",sname:"Balboa High"} (=satscore)
{AvgScrMath:480,AvgScrRead:443,AvgScrWrite:431,cname:"San Francisco",dname:"San Francisco Unified",sname:"Burton (Phillip and Sala) Academic High"} (satscore)
{AvgScrMath:413,AvgScrRead:410,AvgScrWrite:395,cname:"San Francisco",dname:"San Francisco Unified",sname:"City Arts and Tech High"} (satscore)
...
```

Except when writing the most common searches that leverage only the implicit
`and`, it's generally good practice to use parentheses even when not strictly
necessary, just to make sure your queries clearly communicate their intended
logic.
