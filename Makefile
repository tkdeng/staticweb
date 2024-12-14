build all:
	go mod tidy
	go build -C ./exec -o ../staticweb

install:
	make dependencies
	make build
	sudo cp ./staticweb /usr/bin/staticweb

local:
	make dependencies
	make build

dev:
	make build
	sudo cp ./staticweb /usr/bin/staticweb

deps dependencies:
ifeq (,$(wildcard $(/usr/bin/dnf)))
	sudo dnf install pcre-devel
	sudo dnf install go
else ifeq (,$(wildcard $(/usr/bin/apt)))
	sudo apt install libpcre3-dev
else ifeq (,$(wildcard $(/usr/bin/yum)))
	sudo yum install pcre-dev
endif

clean:
	rm ./staticweb
	sudo rm /usr/bin/staticweb
