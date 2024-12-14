# Static Web

Compile HTML and MD template files together into a static html website.

## Installation

```shell
# install the go module
go get github.com/tkdeng/staticweb

# or install the binary
git clone https://github.com/tkdeng/staticweb.git &&\
cd staticweb &&\
make install &&\
cd ../ && rm -r staticweb

# install into /usr/bin
make install

# install locally (with dependencies)
make local

# build without dependency installation
make build

# install dependencies
make deps

# uninstall htmlc
make clean
```

## Golang Usage

```go

import (
  "github.com/tkdeng/staticweb"
)

func main(){
  // compile directory
  err := staticweb.Compile("./src", "./dist")
}
```

## Binary Usage

```shell
staticweb ./src --out="./dist"
```
