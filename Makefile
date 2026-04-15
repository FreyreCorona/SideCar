# Binary name
BINARY_NAME=sidecar

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet
GOINSTALL=$(GOCMD) install


## Build:
build:
	$(GOBUILD) -o $(BINARY_NAME) -v .

## Execution Modes:

# Run in UI mode (default)
ui:
	$(GOCMD) run . -mode ui

# Run in Daemon mode
# Use: make daemon INTERVAL=10s
daemon:
	$(GOCMD) run . -mode daemon $(if $(INTERVAL),-interval $(INTERVAL)) $(if $(DEVICE),-device $(DEVICE))

# Run in CLI mode
# Use: make cli CMD=on
# Use: make cli CMD=upload FILE=path/to/file TYPE=texture
cli:
	$(GOCMD) run . -mode cli -cmd $(CMD) \
		$(if $(DEVICE),-device $(DEVICE)) \
		$(if $(BRIGHTNESS),-brightness $(BRIGHTNESS)) \
		$(if $(FILE),-file $(FILE)) \
		$(if $(TYPE),-type $(TYPE)) \
		$(if $(REG),-reg $(REG)) \
		$(if $(VAL),-val $(VAL)) \
		$(if $(STR),-str $(STR)) \
		$(if $(PAGE),-page $(PAGE)) \
		$(if $(REGS),-regs "$(REGS)")

## Development:

test:
	$(GOTEST) -v ./...

vet:
	$(GOVET) ./...
