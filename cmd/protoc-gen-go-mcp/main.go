package main

import (
	"flag"
	"fmt"

	"google.golang.org/protobuf/types/pluginpb"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
)

const version = "0.0.1"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("protoc-gen-go-mcp %v\n", version)
		return
	}

	var flags flag.FlagSet

	opts := protogen.Options{
		ParamFunc: flags.Set,
	}

	opts.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL) | uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
		gen.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_PROTO2
		gen.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2023

		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}

			generateFile(gen, f)
		}
		return nil
	})
}
