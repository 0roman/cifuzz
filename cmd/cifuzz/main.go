package main

import (
	"strings"

	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/cmd/root"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func init() {
	fileutil.ForceLongPathTempDir()

	viper.SetEnvPrefix("CIFUZZ")
	viper.AutomaticEnv()
	// need to make CIFUZZ_MY_VAR available as viper.Get("my-var")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
}

func main() {
	root.Execute()
}
