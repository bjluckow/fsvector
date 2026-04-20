BINDIR := bin
FSVECTORD := $(BINDIR)/fsvectord
FSVECTOR  := $(BINDIR)/fsvector
FSCLUSTER := $(BINDIR)/fscluster

.PHONY: build clean tidy $(FSVECTORD) $(FSVECTOR) $(FSCLUSTER)

build: tidy $(FSVECTORD) $(FSVECTOR) $(FSCLUSTER)

$(FSVECTORD):
	@mkdir -p $(BINDIR)
	go build -o $@ ./cmd/fsvectord

$(FSVECTOR):
	@mkdir -p $(BINDIR)
	go build -o $@ ./cmd/fsvector

$(FSCLUSTER):
	@mkdir -p $(BINDIR)
	go build -o $@ ./cmd/fscluster

clean:
	rm -rf $(BINDIR)

tidy:
	go mod tidy
