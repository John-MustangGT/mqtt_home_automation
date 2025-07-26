BINNAME = $(shell basename ${PWD})

build: *.go
	go build -o $(BINNAME) .

install: *.go
	go install .

clean:
	rm -f $(BINNAME)
