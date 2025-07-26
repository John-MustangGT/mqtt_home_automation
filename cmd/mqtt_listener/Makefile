BINNAME = $(shell basename ${PWD})

build:
	go build -o $(BINNAME) main.go

install:
	go install main.go

clean:
	rm -f $(BINNAME)

# Example targets for testing
test-xml: build
	./$(BINNAME) -L mqtt://localhost/host1 --config commands.xml

test-legacy: build
	./$(BINNAME) -L mqtt://localhost/host1 --cmd "ping -c 4 1.1.1.1"

.PHONY: build install clean test-xml test-legacy