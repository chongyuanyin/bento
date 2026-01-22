package main

import (
	"context"

	// import all components with:
	// _ "github.com/warpstreamlabs/bento/public/components/all"

	// or you can import select components with:
	// _ "github.com/warpstreamlabs/bento/public/components/aws"
	// for example.

	// io + pure contain components such as stdin/stdout & mapping
	// _ "github.com/warpstreamlabs/bento/public/components/io"
	// _ "github.com/warpstreamlabs/bento/public/components/pure"

	"github.com/warpstreamlabs/bento/public/service"

	// import your plugins:
	_ "flowmq/plugin/dolphindb"
	_ "flowmq/plugin/iotdb"
	_ "github.com/warpstreamlabs/bento/public/components/kafka"
	_ "github.com/warpstreamlabs/bento/public/components/prometheus"
	_ "github.com/warpstreamlabs/bento/public/components/pure"
	// _ "github.com/warpstreamlabs/bento/public/components/mqtt"
)

var (
	// Version version set at compile time.
	Version string
	// BinaryName binary name.
	BinaryName string = "bento"
)

func main() {

	// RunCLI accepts a number of optional functions:
	// https://pkg.go.dev/github.com/warpstreamlabs/bento/public/service#CLIOptFunc
	service.RunCLI(context.Background())
}
