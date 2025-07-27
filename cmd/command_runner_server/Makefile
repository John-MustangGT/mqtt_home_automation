BINNAME = $(shell basename ${PWD})

build:
	go build -o $(BINNAME) main.go

install:
	go install main.go

clean:
	rm -f $(BINNAME)

.PHONY: build install clean
