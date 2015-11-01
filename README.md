[![Build Status](https://travis-ci.org/nnev/kasse.svg?branch=master)](https://travis-ci.org/nnev/kasse)

## Kasse

This is an Implementation of a payment system for the
[Heidelberg Chaostreff](https://www.noname-ev.de). It is currently in a basic
stage of development, so this README.md should only give you enough information to
install it and start testing.

## Installing

Currently you need to install libnfc to build kasse. This will not be needed in
the future. In Debian you need to install the following packages:

- libnfc-dev
- sqlite3
- golang

The following will build the code and give you a basic environment to run kasse:

```
export GOROOT=$HOME/go
export PATH=$PATH:$GOROOT/bin
go get -insecure -u github.com/nnev/kasse`
cd ~/go/src/github.com/nnev/kasse && sqlite3 kasse.sqlite < schema.sql
```

## Testing

It is important, that the binary runs in the path containing kasse.sqlite from
above or gives the full path to it with -connect

`kasse -hardware=false`

## Contributing

Thank you for considering contributing to this repository. Please see our
[contribution guidelines](CONTRIBUTING.md) for some advice on how to make your
contribution as useful as possible.
