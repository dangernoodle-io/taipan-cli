module github.com/dangernoodle-io/taipan-cli

go 1.26.1

require (
	github.com/fatih/color v1.19.0
	github.com/grandcat/zeroconf v1.0.0
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	go.bug.st/serial v1.6.2
	gopkg.in/yaml.v3 v3.0.1
	tinygo.org/x/espflasher v0.4.1-0.20260402180359-0c5b1c5a96fc
)

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/creack/goselect v0.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.27 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/crypto v0.28.0 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
)

replace tinygo.org/x/espflasher => github.com/jgangemi/espflasher v0.0.0-20260406201435-19d929675d82
