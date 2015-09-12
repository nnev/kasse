## Kasse

This is an Implementation of a payment system for the
[Heidelberg Chaostreff](https://www.noname-ev.de). It is currently in a basic
stage of devellopment, so this README.md should give you enough information to
install it and start testing.

## Installing

Currently you need to install libnfc to build kasse. This will not be needed in
the future. In Debian you need to install the following packages:

- libnfc-dev
- sqlite3
- golang


```
export GOROOT=$HOME/go
export PATH=$PATH:$GOROOT/bin
go get -insecure -u github.com/nnev/kasse`
cd ~/go/src/github.com/nnev/kasse && sqlite3 kasse.sqlite < schema.sql
```

## Testing
`kasse --hardware=false`
