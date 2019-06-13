envconfig
=========

[![Build Status](https://travis-ci.org/vrischmann/envconfig.svg?branch=master)](https://travis-ci.org/vrischmann/envconfig)
[![GoDoc](https://godoc.org/github.com/vrischmann/envconfig?status.svg)](https://godoc.org/github.com/vrischmann/envconfig)

envconfig is a library which allows you to parse your configuration from environment variables and fill an arbitrary struct.

See [the example](https://godoc.org/github.com/vrischmann/envconfig#example-Init) to understand how to use it, it's pretty simple.

Supported types
---------------

  * Almost all standard types plus `time.Duration` are supported by default.
  * Slices and arrays
  * Arbitrary structs
  * Custom types via the [Unmarshaler](https://godoc.org/github.com/vrischmann/envconfig/#Unmarshaler) interface.

How does it work
----------------

*envconfig* takes the hierarchy of your configuration struct and the names of the fields to create a environment variable key.

For example:

```go
var conf struct {
    Name string
    Shard struct {
        Host string
        Port int
    }
}
```

This will check for those 3 keys:

  * NAME or name
  * SHARD\_HOST, or shard\_host
  * SHARD\_PORT, or shard\_port

Flexible key naming
-------------------

*envconfig* supports having underscores in the key names where there is a _word boundary_. Now, that term is not super explicit, so let me show you an example:

```go
var conf struct {
    Cassandra struct {
        SSLCert string
        SslKey string
    }
}
```

This will check all of the following keys:

  * CASSANDRA\_SSL\_CERT, CASSANDRA\_SSLCERT, cassandra\_ssl\_cert, cassandra\_sslcert
  * CASSANDRA\_SSL\_KEY, CASSANDRA\_SSLKEY, cassandra\_ssl\_key, cassandra\_sslkey

If that is not good enough, look just below.

Custom environment variable names
---------------------------------

*envconfig* supports custom environment variable names:

```go
var conf struct {
    Name string `envconfig:"myName"`
}
```

Default values
--------------

*envconfig* supports default values:

```go
var conf struct {
    Name string `envconfig:"default=Vincent"`
}
```

Optional values
---------------

*envconfig* supports optional values:

```go
var conf struct {
    Name string `envconfig:"optional"`
}
```

Skipping fields
---------------

*envconfig* supports skipping struct fields:
```go
var conf struct {
    Internal string `envconfig:"-"`
}
```

Combining multiple options in one tag
-------------------------------------

You can of course combine multiple options:

```go
var conf struct {
    Name string `envconfig:"default=Vincent,myName"`
}
```

Slices or arrays
----------------

With slices or arrays, the same naming is applied for the slice. To put multiple elements into the slice or array, you need to separate
them with a *,* (will probably be configurable in the future, or at least have a way to escape)

For example:

```go
var conf struct {
    Ports []int
}
```

This will check for the key __PORTS__:

  * if your variable is *9000* the slice will contain only 9000
  * if your variable is *9000,100* the slice will contain 9000 and 100

For slices of structs, it's a little more complicated. The same splitting of slice elements is done with a *comma*, however, each token must follow
a specific format like this: `{<first field>,<second field>,...}`

For example:

```go
var conf struct {
    Shards []struct {
        Name string
        Port int
    }
}
```

This will check for the key __SHARDS__. Example variable content: `{foobar,9000},{barbaz,20000}`

This will result in two struct defined in the *Shards* slice.

If you want to set default value for slice or array, you have to use `;` as separator, instead of `,`:

```go
var conf struct {
    Ports []int `envconfig:"default=9000;100"`
}
```

Same for slices of structs:

```go
var conf struct {
    Shards []struct {
        Name string
        Port int
    } `envconfig:"default={foobar;localhost:2929};{barbaz;localhost:2828}"`
}
```

Development state
-----------------

I consider _envconfig_ to be pretty much done.

It has been used extensively at [Batch](https://batch.com) for more than 5 years now without much problems,
with no need for new features either.

So, while I will keep maintaining this library (fixing bugs, making it compatible with new versions of Go and so on) for
the foreseeable future, I don't plan on adding new features.

But I'm open to discussion so if you have a need for a particular feature we can discuss it.
