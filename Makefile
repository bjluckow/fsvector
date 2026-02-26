BINDIR := bin
FSVECTORD := $(BINDIR)/fsvectord
FSVECTOR  := $(BINDIR)/fsvector

.PHONY: build clean tidy $(FSVECTORD) $(FSVECTOR)

build: tidy $(FSVECTORD) $(FSVECTOR)

$(FSVECTORD):
	@mkdir -p $(BINDIR)
	go build -o $@ ./cmd/fsvectord

$(FSVECTOR):
	@mkdir -p $(BINDIR)
	go build -o $@ ./cmd/fsvector

clean:
	rm -rf $(BINDIR)

tidy:
	go mod tidy
